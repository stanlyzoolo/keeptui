# Plan: CLI/TUI Registry Refactor

## Overview

Концептуальный сдвиг проекта: из справочника горячих клавиш в персональный реестр CLI/TUI инструментов.

Пользователь добавляет инструменты вручную через `keys track <tool> --github <repo>`. Данные об инструменте (описание, stars, languages, релизы) подтягиваются из GitHub API. Вместо хранимых хоткеев — вывод `<tool> --help` и `man <tool>` прямо в TUI.

**Ключевые изменения:**
- Удаляются: YAML-конфиги инструментов, tldr-интеграция, subcommands new/edit/import/validate/fetch/check
- `ToolMeta` становится единственным источником правды (добавляется поле `github`)
- Правая панель делится на две половины: Tool Card (GitHub-данные) + Help Output
- Левая панель: только My Tools, `/` поиск по именам с автопредпросмотром в правой панели
- Поиск внутри `--help`/`man` вывода: `/` при фокусе на правой половине, `n`/`N` по совпадениям

## Validation Commands
- `go build .`
- `go vet ./...`
- `go test ./...`

---

### Task 1: Удаление старой архитектуры

- [x] Удалить `internal/cmd/fetch.go`
- [x] Удалить `internal/cmd/new.go`
- [x] Удалить `internal/cmd/edit.go`
- [x] Удалить `internal/cmd/import.go`
- [x] Удалить `internal/cmd/validate.go`
- [x] Удалить `internal/cmd/check.go`
- [x] Удалить `internal/loader/validate.go`
- [x] Удалить `internal/tldr/cache.go` и `internal/tldr/parse.go`
- [x] Удалить `internal/loader/data/` (embedded YAML-конфиги)
- [x] Удалить из `loader.Tool`: поля `Categories`, `CommandGroups`; типы `Binding`, `Category`, `Command`, `CommandGroup`; директиву `//go:embed`
- [x] Удалить из `internal/cmd/helpers.go`: `userToolsDir()`, `userToolPath()`, `openEditor()`, `editAndValidate()`, `confirmOverwrite()`
- [x] Удалить из `main.go` case-блоки: `new`, `import`, `edit`, `validate`, `fetch`, `check`; обновить `helpText`
- [x] Удалить из `model.go` весь код, связанный с `Categories`, `CommandGroups`, `tabKeys/tabCommands`, `rightTab`, `renderTool()`, `renderCommandsTab()`, `showPopup`, `renderPopup()`
- [x] Убедиться, что `go build .` проходит без ошибок

### Task 2: Новая модель данных Registry

- [x] Добавить поле `GitHub string` в `loader.ToolMeta` (`meta.go`)
- [x] Переписать `loader.Load()` — строить `[]Tool` из `meta.yaml` вместо embedded YAML
- [x] Обновить `internal/cmd/track.go`: добавить флаг `--github`, сохранять в `ToolMeta.GitHub`
- [x] Обновить `helpText` в `main.go`: `keys track <tool> [--github <repo>] [--status ...] [--tags ...] [--note "..."]`
- [x] Обновить `main.go`: убрать отдельный `loader.Load()`, передавать только `meta` в модель
- [x] Убедиться, что `keys track bat --github github.com/sharkdp/bat` сохраняется корректно
- [x] Убедиться, что `keys list` отображает инструменты из `meta.yaml`

### Task 3: Расширение GitHub API

- [x] Добавить в `version.CacheEntry` поля: `About string`, `Stars int`, `Languages map[string]int`
- [x] Расширить `fetchRepoInfo()`: парсить `description` и `stargazers_count` из `/repos/{owner}/{repo}`
- [x] Добавить `fetchLanguages()`: запрос к `/repos/{owner}/{repo}/languages`, кешировать результат
- [x] Создать тип `version.RepoCard` с полями: `About`, `Stars`, `Languages`, `Latest`, `PublishedAt`, `HtmlUrl`, `Body`, `RepoStatus`
- [x] Создать `version.GetRepoCard(githubField string) RepoCard` — читает из кеша или запускает fetch
- [x] Добавить в `Model` поле `repoCards map[string]version.RepoCard`
- [x] Добавить `repoCardMsg` и `fetchRepoCardCmd()` в `model.go`; запускать в `Init()` для каждого инструмента с GitHub
- [x] Добавить вспомогательные функции рендеринга: `formatStars()`, `languagePercents()`, `renderLangBar()`

### Task 4: Переделка левой панели

- [x] Удалить `viewHotkeys`/`viewMyTools` — один view-режим
- [x] Удалить Top Tabs (`[Hotkeys]  My Tools`) из `renderLeft()`
- [x] Удалить `renderMyToolsList()`, `renderMyToolsDetail()`
- [x] Удалить из `Model`: `view`, `metaDetail`, `editingNote`, `editingTags` (редактирование переедет в правую панель)
- [x] Переписать `renderLeft()`: показывать только tracked tools с символом статуса; footer `N tools`
- [x] Реализовать поиск `/` в левой панели: `searching bool` + `search textinput.Model`
- [x] Расширить `filteredMeta()`: при `searching == true` фильтровать по `strings.Contains(name, query)`
- [x] При изменении поискового запроса — сбрасывать `metaSelected = 0` и запускать загрузку Tool Card + Help для первого результата
- [x] Реализовать клавиши фильтра: `f` (цикл), `1`/`2`/`3`/`4`, `a` (сброс)
- [x] Показывать пустое состояние: `No tools tracked. Add one: keys track <tool> --github ...`

### Task 5: Переделка правой панели

- [ ] Добавить в `Model`: `cardViewport viewport.Model`, `helpViewport viewport.Model`
- [ ] Вычислять ширину: `cardWidth = rightTotal / 2`, `helpWidth = rightTotal - cardWidth`
- [ ] Реализовать `renderRightHeader()`: имя инструмента + статус-символ (без GitHub/версий в заголовке)
- [ ] Реализовать `renderCard()`: блоки About, Stars + Release, Languages bar, Note + Tags, Changelog — разделённые horizontal divider
- [ ] Реализовать инлайн-редактирование Note (`e`) и Tags (`t`) прямо в `renderCard()` через `textinput`
- [ ] Реализовать `renderRight()`: два `viewport` рядом через `lipgloss.JoinHorizontal`
- [ ] Реализовать клавишу `o` (открыть GitHub в браузере) при фокусе на правой панели
- [ ] Удалить `renderChangelog()` и `showChangelog` — changelog теперь в блоке Tool Card
- [ ] Обновить `calcVpHeight()` и расчёты высоты под два viewport

### Task 6: Вывод `--help` и `man`, поиск по выводу

- [ ] Добавить в `Model`: `helpMode int`, `helpLoading bool`, `helpCache map[string][2]string`, `helpSearching bool`, `helpSearch textinput.Model`, `helpMatches []int`, `helpMatchIdx int`
- [ ] Реализовать `fetchHelpCmd(name string, mode int) tea.Cmd`: запускать `--help`/`-h` или `man` с таймаутом 5с; `MANPAGER=cat MANWIDTH=80` для man
- [ ] Реализовать `stripANSI(s string) string` через regexp `\x1b\[[0-9;]*[a-zA-Z]`
- [ ] Обработать `helpOutputMsg` в `Update()`: заполнить `helpCache`, установить контент `helpViewport`; при ошибке — показать сообщение `"--help not available"` / `"man page not available"`
- [ ] Запускать `fetchHelpCmd` автоматически при смене инструмента в левой панели (проверять кеш перед запуском)
- [ ] Клавиши `h` (--help) и `m` (man): переключать `helpMode`, брать из кеша или запускать fetch
- [ ] Реализовать поиск `/` при фокусе на правой половине: активировать `helpSearch`, вызывать `findMatches()` при каждом изменении запроса
- [ ] Реализовать `findMatches(text, query string) []int` — возвращает номера строк с совпадениями
- [ ] Реализовать `highlightMatch(line, query string) string` — подсвечивать совпадение через `SearchMatchStyle`
- [ ] Клавиши `n`/`N` — следующее/предыдущее совпадение через `helpViewport.SetYOffset(helpMatches[helpMatchIdx])`
- [ ] Обновить Help Bar: нормальный режим `[h] --help  [m] man  [/] search`; режим поиска `/ query_  [n] next  [N] prev  [Esc] exit  N matches`
