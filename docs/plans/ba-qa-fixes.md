# Plan: BA/QA Fixes — верстка, dead code, технические баги

## Overview

Исправление проблем, выявленных в ходе BA + QA анализа текущего состояния TUI.

Три группы правок:
1. **Верстка** — word-wrap карточки и хелп-панели, скролл левой панели.
2. **BA** — автозагрузка changelog, подсветка флагов в `--help`.
3. **Cleanup + Tech** — удаление мёртвого кода (`overlay.go`, неиспользуемые стили), mutex на `cache.json`.

## Validation Commands
- `go build .`
- `go vet ./...`
- `go test ./...`

---

### Task 1: Word-wrap контента карточки

- [x] В `renderCard()` определить рабочую ширину `inner := cardW - 2` (за вычетом паддинга рамки)
- [x] Применить `wrapText(card.About, inner)` вместо прямого `ui.DescStyle.Render(card.About)`
- [x] Применить `wrapText("↗ https://"+t.GitHub, inner)` для GitHub URL
- [x] Исправить `wrapText()`: заменить `len(word)` на `utf8.RuneCountInString(word)` — корректная работа с Unicode
- [x] Убедиться, что changelog body тоже оборачивается через исправленный `wrapText`
- [x] Проверить визуально: About, URL и changelog не обрезаются на полуслове (manual test - skipped, not automatable)

### Task 2: Word-wrap хелп-панели

- [x] При заполнении `helpCache` (в обработчике `helpOutputMsg`) применять `wrapText(stripped, helpW-2)` перед передачей в viewport
- [x] Убедиться, что `wrapText` применяется для обоих режимов: `--help` и `man`
- [x] Проверить визуально: длинные строки `--help` не уходят за правый край панели (manual test - skipped, not automatable)

### Task 3: Скролл левой панели

- [x] Заменить `strings.Builder` в `renderLeft()` на `viewport.Model` (`leftViewport`)
- [x] Добавить `leftViewport viewport.Model` в `Model`; инициализировать в `New()` с правильными `Width`/`Height`
- [x] Пересчитывать высоту `leftViewport` в `WindowSizeMsg` (аналогично `cardViewport`/`helpViewport`)
- [x] Передавать `KeyDown`/`KeyUp`/`KeyPgDown`/`KeyPgUp` в `leftViewport`, когда фокус на левой панели
- [x] Синхронизировать видимый диапазон с `metaSelected`: при выборе инструмента за пределами viewport сдвигать `YOffset` так, чтобы выбранный элемент был виден
- [x] Убедиться, что рамка вокруг левой панели не обрезает контент (manual test - skipped, not automatable)

### Task 4: Автозагрузка Changelog

- [x] Удалить строку `"Press [c] to load changelog"` из `renderCard()`
- [x] В `fetchRepoCardCmd()` — включить загрузку changelog body сразу (убрать lazy-load через `[c]`)
- [x] Если загрузка changelog занимает время — показывать `"Loading changelog…"` вместо пустого блока
- [x] Убедиться, что при смене инструмента в левой панели changelog сбрасывается и загружается заново
- [x] Проверить, что пустой блок changelog не занимает лишнего вертикального пространства когда данных нет (manual test - skipped, not automatable)

### Task 5: Подсветка флагов и команд в `--help` выводе

- [x] Написать `colorizeHelp(s string) string`: применять Lip Gloss стили к строкам через regexp
  - Флаги (`-f`, `--flag`) → один цвет
  - Заголовки секций (строка без ведущих пробелов, заканчивается `:`) → другой цвет
  - Метавары в `<angle>` и `[brackets]` → третий цвет
- [x] Добавить стили `HelpFlagStyle`, `HelpSectionStyle`, `HelpMetaStyle` в `internal/ui/styles.go`
- [x] Применять `colorizeHelp()` после `wrapText()` в `renderHelpContent()` (только вне режима поиска)
- [x] Убедиться, что поиск (`findMatches`, `highlightMatch`) работает по plain-text версии, а не по ANSI-строкам
- [x] Проверить визуально на `bat --help` и `yazi --help` (manual test - skipped, not automatable)

### Task 6: Удаление мёртвого кода

- [x] Удалить `internal/ui/overlay.go` (`Overlay()`, `PlaceOverlay()` — не вызываются после удаления popup)
- [x] Удалить из `internal/ui/styles.go` неиспользуемые переменные:
  `SelectedBindingStyle`, `BindingCountStyle`, `ToolSelectedStyle`, `ToolNormalStyle`,
  `CategoryStyle`, `KeyStyle`, `HeaderDescStyle`,
  `TabActiveStyle`, `TabInactiveStyle`,
  `PopupStyle`, `ChangelogPopupStyle`,
  `CommandCmdStyle`, `CommandDescStyle`, `CommandCountStyle`,
  `TopTabActiveStyle`, `TopTabInactiveStyle`,
  `ColorBg`, `ColorSelected`
- [x] Убедиться, что `go build .` и `go vet ./...` проходят без ошибок после удаления

### Task 7: Mutex на cache.json

- [x] Добавить `var cacheMu sync.Mutex` в `internal/version/github.go`
- [x] Обернуть `LoadCache` + мутацию + `SaveCache` в `cacheMu.Lock()` / `cacheMu.Unlock()` внутри `FetchAndCache()`
- [x] Убедиться, что `Init()` в `model.go` по-прежнему запускает горутины параллельно — только запись сериализована
- [x] Написать тест `TestConcurrentFetch` (3+ инструмента, параллельный вызов `FetchAndCache`): убедиться, что итоговый кеш содержит записи для всех инструментов
