# UI Revision: Tools List Panel, Card Versions, Panel Titles, Search

## Overview

UI-ревизия TUI по итогам brainstorm-сессии (кадры снимались с живого приложения через tmux). Четыре согласованные секции:

- **A. Видимость выделения** — маркер выделения не исчезает при переносе фокуса на brief/help (сейчас `renderLeftContent` рисует его только при `focus == focusTools`, и невозможно понять, чей brief открыт); глиф меняется с `●` на `▸`, выбранное имя выделяется жирным при активном фокусе.
- **B. Статусы и версия** — статус `forgotten` удаляется, `archived` переименовывается в `inactive` (цвет `ColorMuted` вместо невидимого на тёмной теме `ColorBorder`); в списке не-active строки помечаются левой кромкой `▎`; в `[info]` карточки появляется строка `installed:`, а `latest:` подсвечивается цветом обновления, когда установленная версия старше.
- **C. Заголовок help-панели** — титул `--help`/`man` врезается в верхнюю рамку (режим панели сейчас никак не обозначен). Brief остаётся без титула. Вариант `tools · 17` отложен по решению пользователя.
- **D. Поиск** — матч по тегам в дополнение к имени, подсветка совпавшей подстроки в списке, тускло показанный тег у строк, совпавших только тегом, счётчик `3/17` в статус-баре.

## Context (from discovery)

- `internal/model/render.go` — `renderLeftContent` (~380), `renderTools`/`renderBrief`/`renderHelp` (~436–470), `renderCard` (~509, `latest:` на ~566), `hasUpdate` (~681), статус-бар поиска (~55)
- `internal/model/model.go` — `filteredMeta` (~774), `m.versions` (installed/latest, заполняется `installedMsg`/`remoteMsg`)
- `internal/loader/meta.go` — `Status`-константы (~13–25), `StatusSymbol`, `StatusCycle`, `NextStatus` (~125), `LoadMeta` (~57, точка миграции)
- `internal/ui/styles.go` (~21–94: `SelectionBarStyle`, `StatusColor*`, `StatusStyle*`), `internal/ui/status.go` (`StatusStyle` switch)
- `internal/model/textutil.go` — `stripANSI` (~168), `findMatches` для подсветки матчей; ANSI-обрезка `truncateVisible`/`dropVisible` — **неэкспортируемые в `internal/ui/overlay.go:75,80`**, из `model` недоступны (для врезки титула нужен локальный хелпер)
- Тесты уже есть: `loader/meta_test.go` (в т.ч. `TestNextStatus:170`), `model/render_test.go` (в т.ч. `TestUpdateBriefStatusCycle` с полным циклом статусов на ~988–992 — сломается при удалении forgotten/archived), `model/mode_test.go` — паттерны для новых тестов брать оттуда
- Принципы проекта: foreground-глифы вместо фоновых заливок (деградированные цветовые профили); глифы не из East-Asian-Ambiguous диапазона (`▸` U+25B8 и `▎` U+258E — безопасны)

## Development Approach

- **testing approach**: Regular (code first, then tests) — в духе существующего тест-набора
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional — they are a required part of the checklist
  - unit-тесты для новых и изменённых функций, success + error/edge сценарии
- **CRITICAL: all tests must pass before starting next task** — `go test -race ./...` (в version-пакете реальное mutex-состояние, `-race` обязателен)
- **CRITICAL: update this plan file when scope changes during implementation**
- maintain backward compatibility: существующие `meta.yaml` с `forgotten`/`archived` должны загружаться без ошибок (миграция в `inactive`)

## Testing Strategy

- **unit tests**: обязательны в каждой задаче; рендер-логика проверяется по строковому выводу (со `stripANSI` где нужно), как в существующем `render_test.go`
- **e2e**: UI-based e2e фреймворка в проекте нет; вместо него ручная проверка живьём через tmux (см. Post-Completion)

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

Ключевые решения из brainstorm:

- **Один глиф — один смысл**: `▸` — курсор, `▎` — статусная кромка, `↑` — обновление, `●` остаётся только статусной точкой внутри карточки. Коллизия «персиковая ● = выделение / зелёная ● = active» устраняется.
- **Приглушение вместо исчезновения**: при фокусе не на списке маркер `▸` рисуется dim-стилем — выбранная строка всегда опознаваема.
- **Кромка только у отклонений**: active — норма и не маркируется; `▎` появляется лишь у trying (оранжевая) и inactive (серая). Кромка живёт в колонке маркера; у выбранной строки `▸` приоритетнее.
- **Статусы**: цикл `active → trying → inactive`. `forgotten` — логическое противоречие («забыл — значит не открыл и не поставил»), удаляется. `archived` конфликтовал по имени с archived-репозиторием GitHub (`maintenance:`) и был невидим цветом — переименование + `ColorMuted`.
- **Версии — в карточке, не в списке**: постоянный показ версий в 30-колоночном списке зашумляет его и не выравнивается; `installed:` встаёт в естественную key:value-сетку `[info]`.
- **Поиск — только предикат и рендер**: транзакция commit/rollback (`searchPrevName`, ремап через `indexOfMeta`, enter/esc-семантика) не трогается.

## Technical Details

- **Статусы (loader)**: `StatusInactive Status = "inactive"`; `StatusForgotten`/`StatusArchived` удаляются из констант, `StatusSymbol`, `StatusCycle`. Миграция в `LoadMeta` после unmarshal: `forgotten`→`inactive`, `archived`→`inactive` (простой switch по значению; неизвестные значения не трогать — `NextStatus` уже фолбэчит в active). Символ inactive: `✕` (сохраняется от archived). Миграция — **in-memory**: на диске старые значения живут до ближайшего `SaveMeta`; это осознанно (загрузка без ошибок = обратная совместимость), нужен round-trip тест load→save→reload. **Не трогать `renderRepoStatus` (render.go ~640) и его тест (`{"archived", …⚠ archived}` в render_test.go ~184)** — это maintenance-статус GitHub-репозитория, не статус инструмента.
- **Стили (ui)**: `SelectionBarStyle` (персиковый, для focused) + новый `SelectionBarDimStyle` (`ColorDim`/muted); имя выбранной строки при focused — жирный персиковый (`SelectedNameStyle`). `StatusColorInactive = ColorMuted`; `StatusColorForgotten`, `StatusStyleForgotten`, `StatusColorArchived`, `StatusStyleArchived` удаляются, добавляется `StatusStyleInactive`. Стиль кромки: `EdgeTrying` (оранжевый `▎`), `EdgeInactive` (muted `▎`) — можно переиспользовать статусные цвета напрямую.
- **Строка списка** (`renderLeftContent`): колонка маркера шириной 1 + пробел. Приоритет содержимого колонки: `▸` (выбрана) > `▎` (не-active) > пробел. `maxName` не меняется.
- **Карточка**: `installed:` берётся из `m.versions[name].Installed`; пустое значение → `installed: not found` в `DescStyle`. Подсветка `latest:`: при `hasUpdate(name)` значение версии рендерится `UpdateAvailableStyle` + суффикс ` ↑`; иначе как сейчас. **Внимание на гейт `hasInfo` (render.go:551)**: секция `[info]` сейчас рендерится только при `t.GitHub != ""` или наличии card-данных — у локально установленного инструмента без GitHub-ссылки `installed:` не отрендерится. Расширить предикат (`hasInfo || installed != ""`) либо выводить `installed:` вне GitHub-гейта.
- **Титул рамки**: хелпер `renderPanelTitled(style, w, h, title, content)` (render.go) — рендерит рамку как сейчас, затем врезает ` title ` в верхнюю строку начиная с 3-й видимой ячейки. `truncateVisible`/`dropVisible` из `ui/overlay.go` неэкспортируемые — написать локальный ANSI-безопасный сплайс в `model` (верхняя строка рамки — однородно окрашенные `─`, так что достаточно `stripANSI`-замера + пересборки строки; полноценный ANSI-парсер не нужен). Цвет титула = цвет рамки (фокус/нет). Используется только help-панелью: титул `--help` / `man` по `m.helpMode`.
- **Поиск**: `filteredMeta` матчит `strings.Contains(lower(name))` ИЛИ `strings.Contains(lower(tag))` по любому тегу. Для рендера нужно знать «матч только по тегу» — вернуть рядом с items информацию о матче (например, второй срез или маленькая структура `searchMatch{meta, byTagOnly, tag}`; выбрать по месту минимальный вариант, не ломая существующих вызовов `filteredMeta` — их несколько: подсчёт, выбор, рендер). Подсветка подстроки имени: жирный + персиковый на диапазоне совпадения. Счётчик в статус-баре поиска: `N/M` (N — матчей, M — всего в `m.meta`).

## What Goes Where

- **Implementation Steps** (`[ ]`): код, тесты, документация в этом репозитории
- **Post-Completion** (без чекбоксов): ручная проверка живьём в терминале

## Implementation Steps

### Task 1: Статусы в loader — inactive вместо forgotten/archived + миграция

**Files:**
- Modify: `internal/loader/meta.go`
- Modify: `internal/loader/meta_test.go`
- Modify: `internal/model/render_test.go` (только компиляционный фикс цикла статусов)

- [x] заменить `StatusForgotten`/`StatusArchived` на `StatusInactive = "inactive"` в константах, `StatusSymbol` (`✕`), `StatusCycle` (active → trying → inactive)
- [x] добавить нормализацию в `LoadMeta` после unmarshal: `forgotten`→`inactive`, `archived`→`inactive`
- [x] обновить `TestNextStatus` под новый цикл (включая фолбэк неизвестного статуса → active)
- [x] обновить `TestUpdateBriefStatusCycle` (`render_test.go:988–992`): `want`-срез перечисляет `StatusForgotten`/`StatusArchived` — без правки тест-бинарь пакета `model` не соберётся и гейт не пройдёт
- [x] написать тест миграции: yaml с `status: forgotten` и `status: archived` загружается как `inactive`; `active`/`trying` не изменяются
- [x] написать round-trip тест: load (`forgotten`) → `SaveMeta` → reload → на диске и в памяти `inactive`
- [x] прогнать `go test -race ./...` — компиляция сломается и в `ui/status.go`/`ui/styles.go`; допустимо чинить их минимальной правкой здесь же (полноценные стили — Task 2), тесты должны пройти до перехода дальше (ui/styles.go и ui/status.go починены переименованием forgotten/archived → inactive; golangci-lint в окружении не запускается — несовпадение версий Go, не связано с изменениями)

### Task 2: Стили ui — inactive, dim-выделение, кромка

**Files:**
- Modify: `internal/ui/styles.go`
- Modify: `internal/ui/status.go`

- [x] `StatusColorInactive = ColorMuted`, `StatusStyleInactive`; удалить forgotten/archived стили и цвета (сделано минимальной правкой ещё в Task 1; остатков forgotten/archived в пакете нет)
- [x] обновить switch в `ui/status.go` (`StatusInactive` → `StatusStyleInactive`); default-ветка остаётся `StatusStyleTrying` (как сейчас — «неизвестный статус выглядит как пробуемый»)
- [x] добавить `SelectionBarDimStyle` (приглушённый маркер при потере фокуса) и стиль жирного персикового имени выбранной строки (`SelectedNameStyle`)
- [x] написать/обновить тесты на `StatusStyle` (все три статуса + default-ветка) — новый `internal/ui/status_test.go`
- [x] прогнать `go test -race ./...` — must pass before task 3

### Task 3: Список — маркер ▸, не исчезающий при смене фокуса, кромка ▎

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] в `renderLeftContent` заменить `●` на `▸` и убрать `&& m.focus == focusTools`: маркер персиковый при `focusTools`, dim иначе; имя выбранной строки жирным только при `focusTools`
- [x] колонка маркера: `▸` у выбранной, иначе `▎` в цвете статуса у trying/inactive, иначе пробел (хелпер `statusEdge` в render.go)
- [x] проверить, что `maxName`/ширина строки не поехали (глифы одноколоночные) — тест `TestRenderLeftContentRowWidth` фиксирует видимую ширину строк
- [x] тесты: маркер присутствует при фокусе на brief/help (dim); `▸` у выбранной строки вытесняет её кромку; `▎` у не-active строк; active-строки без кромки (`TestRenderLeftContentMarkerSurvivesFocus`, `TestRenderLeftContentStatusEdge`; существующий `TestRenderLeftContentSearchMarker` переведён на `▸`)
- [x] прогнать `go test -race ./...` — must pass before task 4

### Task 4: Карточка — installed: и подсветка latest:

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] в `renderCard` добавить строку `installed: <m.versions[name].Installed>` рядом с `latest:`; пустая → `installed: not found` тускло
- [x] расширить гейт `hasInfo` (render.go:551), чтобы `installed:` рендерился и у инструмента без GitHub-ссылки (локально установлен, card-данных нет)
- [x] при `hasUpdate(name)` рендерить значение `latest:` стилем `UpdateAvailableStyle` + ` ↑`; без апдейта — как сейчас
- [x] тесты: карточка с installed+latest (равны — без подсветки), с апдейтом (подсветка + ↑), без установленной версии (not found), без данных версий вовсе, инструмент без GitHub но с installed (строка присутствует) — `TestRenderCardInstalledLatest` (+ кейс «нет GitHub и нет installed → секции [info] нет»)
- [x] прогнать `go test -race ./...` — must pass before task 5

### Task 5: Титул help-панели (--help / man) в рамке

**Files:**
- Modify: `internal/model/render.go`
- Modify: `internal/model/render_test.go`

- [x] локальный хелпер врезки титула в верхнюю строку отрендеренной рамки (ANSI-безопасно; `truncateVisible` из `ui/overlay.go` неэкспортируемый — писать свой на базе `stripANSI`, см. Technical Details); титул в цвете рамки (фокус/нет) — `insetPanelTitle` в render.go
- [x] `renderHelp` использует хелпер: титул `--help` или `man` по `m.helpMode`; brief и tools не трогать
- [x] тесты: титул присутствует и соответствует helpMode; ширина верхней строки рамки не изменилась (stripANSI-длина); слишком узкая панель не паникует (титул обрезается/опускается) — `TestRenderHelpTitle`, `TestInsetPanelTitle`
- [x] прогнать `go test -race ./...` — must pass before task 6

### Task 6: Поиск — теги, подсветка матча, счётчик

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render.go`
- Modify: `internal/model/mode_test.go`
- Modify: `internal/model/render_test.go`

- [x] `filteredMeta`: матч по имени ИЛИ по любому тегу (lowercase substring); не ломать существующие вызовы (подсчёт, выбор, ремап курсора) — предикат вынесен в `searchMatches()`, `filteredMeta` — проекция
- [x] прокинуть в рендер признак «матч только по тегу» + сам тег (структура `searchMatch{meta, byTagOnly, tag}` рядом с `filteredMeta` в model.go)
- [x] `renderLeftContent` в `modeSearch`: подсветка совпавшей подстроки имени (жирный + персиковый, `highlightNameMatch` — существующий `highlightMatch` в textutil.go принадлежит help-поиску); у tag-only матчей справа тусклый `#<tag>` (опускается, если не влезает в бюджет строки без переноса)
- [x] статус-бар поиска: счётчик `N/M` после запроса (`/ > tui   3/17  [enter] open …`)
- [x] тесты: матч по тегу попадает в фильтр; tag-only строка помечена тегом; подсветка подстроки в выводе; счётчик в статус-баре (включая `0/M`); commit/rollback-семантика не изменилась (существующие тесты mode_test.go проходят без правок логики) — `TestSearchMatchesByTag`, `TestSearchNameMatchNotTagFlagged`, `TestRenderLeftContentTagMatchSuffix`, `TestRenderLeftContentSearchHighlight`, `TestHighlightNameMatch`, `TestRenderStatusBarSearchCounter`
- [x] прогнать `go test -race ./...` — must pass before task 7

### Task 7: Verify acceptance criteria

- [ ] пройтись по Overview: A (маркер виден при любом фокусе), B (3 статуса, миграция, installed/latest), C (титул help-панели), D (теги, подсветка, счётчик)
- [ ] edge-cases: пустой список, инструмент без GitHub, узкая панель, поиск без матчей
- [ ] `go build .` + `go vet ./...` + `golangci-lint run`
- [ ] полный прогон: `go test -race ./...`
- [ ] живой прогон в tmux (см. Post-Completion) — визуальная сверка кадров с дизайном

### Task 8: [Final] Update documentation

- [ ] обновить CLAUDE.md: разделы про статусы (цикл из трёх), маркер выделения, титул help-панели, охват поиска
- [ ] README.md — если там упоминаются статусы/горячие клавиши, синхронизировать
- [ ] переместить этот план в `docs/plans/completed/`

## Post-Completion

**Manual verification** (живой прогон):

```bash
go build -o /tmp/keys-check . && tmux new-session -d -s keysui -x 160 -y 45 /tmp/keys-check
tmux capture-pane -t keysui -p -e   # цветной кадр
# сценарии: j/k, → (маркер остаётся dim), / + запрос по тегу, esc/enter, [h]/[m] титул, карточка с апдейтом
tmux send-keys -t keysui q; tmux kill-session -t keysui
```

- сверить кромку `▎` и dim-маркер на тёмной и светлой теме терминала
- проверить деградированный профиль (`TERM=xterm` без truecolor) — глифы должны остаться различимыми

**External system updates**: не требуются (локальное TUI-приложение).
