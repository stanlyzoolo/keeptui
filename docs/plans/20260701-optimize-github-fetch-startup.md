# Оптимизация задержки старта при GitHub-фетчах

## Overview
На старте `keys` наполнение панелей ощущается медленным. Причина — не блокировка UI (все `fetch*Cmd` идут в горутинах через `tea.Batch`), а три проблемы в сетевом слое:

1. **Гонка записи `cache.json`** (первопричина). `GetLatest`/`GetRepoCard`/`GetChangelog` делают `LoadCache()` → правят свою запись → `SaveCache()` (перезапись всего файла) **без** `cacheMu`. На старте ~2N горутин стартуют с одного устаревшего снапшота и затирают записи друг друга — «последний писатель побеждает». Из N свежих записей на диске остаётся практически одна → на следующем старте TTL-промах у всех → **полный перефетч каждый раз**.
2. **Дублирующийся `fetchRepoInfo`**. `GetLatest` и `GetRepoCard` оба зовут `fetchRepoInfo` для одного и того же repo, в двух параллельных командах на каждый тул (~4 HTTP/тул, один лишний).
3. **Локальная детекция версии связана с сетью**. `fetchVersionCmd` склеивает подпроцесс `InstalledVersion` (до 2с) и сетевой `GetLatest` в один `versionMsg` — установленная версия не показывается, пока не пройдёт сеть.

**Решаем (scope A+C+B):**
- **A** — сериализация кэша + merge-on-write под `cacheMu` (чинит перефетч и гонку).
- **C** — отделить `InstalledVersion` в свою команду (мгновенный показ установленной версии).
- **B** — слить сетевую часть version+repocard в один per-tool фетч (release + repoInfo + languages за один проход, без дубля).

Вне scope: D (пул воркеров/ленивый фетч) и E (ETag/conditional requests) — отдельно, если упрёмся в rate-limit.

## Context (from discovery)
- Файлы/компоненты:
  - `internal/version/github.go` — `LoadCache`/`SaveCache`, `GetLatest`, `GetRepoCard`, `GetChangelog`, `FetchAndCache`, `fetchRelease`/`fetchRepoInfo`/`fetchLanguages`, `cacheMu`, `cacheTTL`.
  - `internal/version/detect.go` — `InstalledVersion` (локальный подпроцесс).
  - `internal/model/model.go` — `Init()` (161), `autoFetchCmdsForSelected()` (1456), обработчики сообщений (192–216), команды `fetchVersionCmd`/`fetchRepoCardCmd`/`fetchChangelogCmd` (1399–1434), предикаты `needsVersion`/`needsRepoCard` (1436–1451), типы `versionMsg`/`repoCardMsg`/`changelogMsg` (35–55).
- Паттерны:
  - Async-fetch split симметричен: `Init()` фетчит всё для всех, `autoFetchCmdsForSelected()` — для выбранного под guard-предикатами (см. CLAUDE.md).
  - `m.versions map[string]VersionInfo{Installed, Latest}`, `m.repoStatus map[string]string`, `m.repoCards map[string]version.RepoCard`.
  - Только `FetchAndCache` уже держит `cacheMu` — остальные пути кэша нет.
- Зависимости: rename-путь (`model.go:783`) чистит `m.versions`/`m.repoCards`/`m.repoStatus` по старому имени — при добавлении новых сообщений семантика мапов не меняется, чистка остаётся валидной.
- Тесты рядом: `internal/version/github_test.go` (уже проверяет конкурентный `FetchAndCache`), `internal/model/render_test.go`.

## Development Approach
- **testing approach**: Regular (код, затем тесты в той же задаче) — как в текущем репо.
- каждую задачу доводим до конца перед следующей; изменения маленькие и сфокусированные.
- **КРИТИЧНО: каждая задача содержит новые/обновлённые тесты** для изменённого кода (success + error/edge).
- **КРИТИЧНО: все тесты зелёные перед началом следующей задачи.**
- после каждого изменения: `go build ./... && go vet ./... && go test ./...`.
- сохраняем обратную совместимость формата `cache.json` (не меняем структуру `CacheEntry`).

## Testing Strategy
- **unit-тесты**: обязательны в каждой задаче.
  - A: конкурентный тест — N горутин пишут разные repo одновременно, все записи присутствуют в кэше (расширение существующего `github_test.go`).
  - B: `GetRepoData` — один проход возвращает latest+status+card, кэш заполнен, повторный вызов в пределах TTL не ходит в сеть (проверка через счётчик обращений/подменный клиент, если инфраструктура позволяет; иначе — через кэш-хит).
  - C: модель — `installedMsg` и `remoteMsg` мержатся в одну `VersionInfo` без потери полей.
- **e2e**: в проекте нет UI-e2e (чистый TUI, тестируется юнитами рендера) — не добавляем.

## Progress Tracking
- отмечаем `[x]` сразу по завершении пункта.
- новые задачи — с префиксом ➕, блокеры — ⚠️.
- держим план в синхроне с фактической работой.

## Solution Overview
- **A (merge-on-write):** ввести единый write-хелпер `updateCacheEntry(repo, mutate func(CacheEntry) CacheEntry)`, который под `cacheMu` перечитывает актуальный кэш с диска, применяет изменение одной записи и пишет обратно. Все пути записи (`GetLatest`, `GetRepoCard`, `GetChangelog`, `FetchAndCache`) переводим на него — read-modify-write становится атомарным, записи не теряются.
- **B (единый сетевой проход):** новая `GetRepoData(githubField) RepoData` делает `fetchRelease` + `fetchRepoInfo` + `fetchLanguages` **один раз**, обновляет кэш одним `updateCacheEntry`, возвращает совокупность (latest, status, about, stars, languages, changelog-поля). `GetLatest`/`GetRepoCard` остаются как тонкие обёртки над кэшем/`GetRepoData` для совместимости и точечных вызовов.
- **C (split локальной детекции):** в модели `fetchVersionCmd` распадается на `fetchInstalledCmd` (локальный, `installedMsg`) и сетевую часть. Обработчики мержат в существующую `VersionInfo`.
- **Wiring (B+C в модели):** на каждый тул в `Init()`:
  - `fetchInstalledCmd(t)` — всегда (локально, мгновенно);
  - `fetchRemoteCmd(t)` — только если `t.GitHub != ""`; один сетевой проход через `GetRepoData`, эмитит `remoteMsg{latest, repoStatus, card}`, наполняющий `m.versions.Latest` + `m.repoStatus` + `m.repoCards`.
  - `autoFetchCmdsForSelected()` — те же команды под обновлёнными guard'ами.

## Technical Details
- **Новый хелпер (github.go):**
  ```go
  func updateCacheEntry(repo string, mutate func(existing CacheEntry) CacheEntry) {
      cacheMu.Lock()
      defer cacheMu.Unlock()
      cache := LoadCache()          // свежий снапшот с диска
      cache[repo] = mutate(cache[repo])
      SaveCache(cache)
  }
  ```
  `mutate` строит новую запись, беря недостающие поля из `existing` (свежего, не из устаревшего снапшота вызывающей горутины).
- **RepoData (github.go):**
  ```go
  type RepoData struct {
      Latest, RepoStatus, About string
      Stars int
      Languages map[string]int
      Body, HtmlUrl, PublishedAt string
  }
  func GetRepoData(githubField string) RepoData // release+repoInfo+languages, 1 проход, кэш через updateCacheEntry
  ```
- **Сообщения (model.go):**
  - `installedMsg{toolName, installed string}` → мержит `Installed` в `m.versions[toolName]` (сохраняя `Latest`).
  - `remoteMsg{toolName, latest, repoStatus string, card version.RepoCard, err error}` → мержит `Latest`, пишет `m.repoStatus`, `m.repoCards`.
  - `versionMsg`/`repoCardMsg` удаляются после переезда всех вызовов (или оставляются только если ещё используются changelog-путём — проверить).
- **Guard'ы (model.go):**
  - `needsInstalled(t)` — нет записи `Installed` в `m.versions` (не гоняем подпроцесс на каждое движение курсора).
  - `needsRemote(t)` — `t.GitHub != ""` и (нет `Latest` или нет карточки). Заменяет пару `needsVersion`/`needsRepoCard`.
- Формат `cache.json` и `CacheEntry` не меняются — обратная совместимость сохранена.
- `GetChangelog` остаётся отдельной командой (ленивый, только для выбранного) — но переводится на `updateCacheEntry` (часть A).

## What Goes Where
- **Implementation Steps** (чекбоксы): изменения в `internal/version` и `internal/model`, их тесты, правка CLAUDE.md.
- **Post-Completion** (без чекбоксов): ручная проверка старта на реальном наборе тулов и поведения под rate-limit.

## Implementation Steps

### Task 1: Атомарный write-хелпер кэша `updateCacheEntry` (A)

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [x] добавить `updateCacheEntry(repo string, mutate func(CacheEntry) CacheEntry)` — под `cacheMu`: `LoadCache` → `mutate(cache[repo])` → `SaveCache`.
- [x] написать тест: M горутин параллельно зовут `updateCacheEntry` для M разных repo → после — в кэше присутствуют все M записей (нет потерянных).
- [x] написать тест: конкурентные обновления одного repo не теряют последнее значение (сериализация под мьютексом).
- [x] `go test ./internal/version/` — зелёные перед следующей задачей.

### Task 2: Перевод всех путей записи кэша на `updateCacheEntry` (A)

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [x] переписать `GetLatest`: вместо `cache[repo]=...; SaveCache(cache)` — `updateCacheEntry`, поля отсутствующие в свежей записи брать из `existing` (сохранить fallback `RepoStatus`/`Languages`).
- [x] переписать `GetRepoCard` аналогично через `updateCacheEntry`.
- [x] переписать `GetChangelog` аналогично через `updateCacheEntry`.
- [x] привести `FetchAndCache` к `updateCacheEntry` (убрать ручной `cacheMu.Lock`/дублирование логики).
- [x] обновить/добавить тесты: чтение-после-записи для каждого пути; параллельный `Init`-подобный сценарий (release+card на несколько repo) сохраняет все записи.
- [x] `go test ./internal/version/` — зелёные перед следующей задачей.

### Task 3: Единый сетевой проход `GetRepoData` (B)

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [x] добавить тип `RepoData` и функцию `GetRepoData(githubField)`: кэш-хит при свежем TTL и заполненных полях; иначе `fetchRelease` + `fetchRepoInfo` + `fetchLanguages` **один раз**, запись через `updateCacheEntry`.
- [x] переориентировать `GetLatest`/`GetRepoCard` на общий путь (обёртки над кэшем/`GetRepoData`), убрав повторный `fetchRepoInfo`.
- [x] написать тест: `GetRepoData` возвращает latest+status+about+stars+languages за один вызов; повторный вызов в пределах TTL не инициирует сетевые вызовы (проверка кэш-хита).
- [x] написать тест на edge: пустой/невалидный `githubField` → пустой `RepoData` без паники.
- [x] `go test ./internal/version/` — зелёные перед следующей задачей.

### Task 4: Split команд модели — `installedMsg` + `remoteMsg` (C + wiring B)

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [ ] добавить типы `installedMsg` и `remoteMsg`; добавить `fetchInstalledCmd(t)` (локальный `InstalledVersion`) и `fetchRemoteCmd(t)` (через `version.GetRepoData`).
- [ ] обработчики в `Update`: `installedMsg` мержит `Installed` в существующую `VersionInfo`; `remoteMsg` мержит `Latest` + пишет `m.repoStatus` + `m.repoCards` (сохранять уже имеющиеся поля).
- [ ] обновить `Init()`: на тул — `fetchInstalledCmd` всегда, `fetchRemoteCmd` при `t.GitHub != ""`; help/changelog для выбранного оставить как есть.
- [ ] заменить `needsVersion`/`needsRepoCard` на `needsInstalled`/`needsRemote`; обновить `autoFetchCmdsForSelected()` на новые команды/guard'ы.
- [ ] удалить неиспользуемые `versionMsg`/`repoCardMsg`/`fetchVersionCmd`/`fetchRepoCardCmd` (проверив, что нигде не остались ссылки; rename-путь на `model.go:783` не трогаем — мапы те же).
- [ ] написать тесты: `installedMsg` затем `remoteMsg` (и наоборот) дают полную `VersionInfo{Installed, Latest}` без потери полей; `remoteMsg` наполняет `repoCards`/`repoStatus`; тул без `GitHub` получает только `Installed`.
- [ ] `go build ./... && go vet ./... && go test ./...` — зелёные перед следующей задачей.

### Task 5: Verify acceptance criteria
- [ ] версии/статусы/карточки по-прежнему корректно отображаются (юнит-тесты рендера зелёные).
- [ ] `cache.json` после параллельного старта содержит записи по всем repo (нет потерь) — покрыто тестами Task 1–2.
- [ ] на каждый тул нет повторного `fetchRepoInfo` (проверка по коду/тесту Task 3).
- [ ] установленная версия рендерится независимо от сети (Task 4).
- [ ] полный прогон: `go build ./... && go vet ./... && go test ./...`.

### Task 6: [Final] Update documentation
- [ ] обновить CLAUDE.md: описать новый async-fetch split (`fetchInstalledCmd`/`fetchRemoteCmd`, `installedMsg`/`remoteMsg`, `needsInstalled`/`needsRemote`) и правило записи кэша через `updateCacheEntry` под `cacheMu`.
- [ ] обновить README.md при необходимости (вряд ли нужно — внутренняя механика).
- [ ] переместить этот план в `docs/plans/completed/`.

## Post-Completion
*Требует ручного/внешнего действия — без чекбоксов, информационно.*

**Ручная проверка:**
- запустить `keys` на реальном наборе тулов (холодный кэш → тёплый): убедиться, что второй старт заметно быстрее (записи кэша сохранились).
- проверить поведение при исчерпании GitHub rate-limit без `GITHUB_TOKEN`: фолбэк на stale-кэш, отсутствие «мерцания» перефетча на каждом старте.

**Возможное продолжение (вне scope):**
- D — пул воркеров / ленивый сетевой фетч для невыбранных тулов при большом их числе.
- E — conditional requests (ETag / `If-None-Match`) для экономии rate-limit.
