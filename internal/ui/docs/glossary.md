# UI/UX Глоссарий — keepkeys

Справочник по именованию элементов интерфейса для использования в задачах, доработках и обсуждениях.

## Содержание

- [Верхнеуровневые представления (Views)](#верхнеуровневые-представления-views)
- [Глобальный макет (Hotkeys View)](#глобальный-макет-hotkeys-view)
- [Панели (Panels)](#панели-panels)
  - [Left Panel — Левая панель](#left-panel--левая-панель)
  - [Right Panel — Правая панель](#right-panel--правая-панель)
- [Элементы Right Panel](#элементы-right-panel)
  - [Header — Заголовок правой панели](#header--заголовок-правой-панели)
  - [Inner Tabs — Внутренние табы](#inner-tabs--внутренние-табы)
  - [Viewport — Область прокрутки](#viewport--область-прокрутки)
- [Содержимое Viewport](#содержимое-viewport)
  - [Category — Категория](#category--категория)
  - [Binding Row — Строка биндинга](#binding-row--строка-биндинга)
  - [Command Row — Строка команды](#command-row--строка-команды)
  - [Selection Bar — Индикатор выбора](#selection-bar--индикатор-выбора)
- [Help Bar — Строка подсказок](#help-bar--строка-подсказок)
  - [Key Hint — Подсказка клавиши](#key-hint--подсказка-клавиши)
- [Top Tabs — Верхние табы](#top-tabs--верхние-табы)
- [Оверлеи (Overlays / Popups)](#оверлеи-overlays--popups)
  - [Command Popup — Попап команды](#command-popup--попап-команды)
  - [Changelog Popup — Попап чейнджлога](#changelog-popup--попап-чейнджлога)
- [My Tools View](#my-tools-view)
  - [My Tools List — Список инструментов](#my-tools-list--список-инструментов)
  - [My Tools Detail — Детали инструмента](#my-tools-detail--детали-инструмента)
  - [Filter Bar — Строка фильтра](#filter-bar--строка-фильтра)
  - [Stats Footer — Итоговая строка статистики](#stats-footer--итоговая-строка-статистики)
  - [Status Symbol — Символ статуса](#status-symbol--символ-статуса)
- [Статусы инструментов (My Tools)](#статусы-инструментов-my-tools)
- [Search Mode — Режим поиска](#search-mode--режим-поиска)
- [Цветовая палитра](#цветовая-палитра)

---

## Верхнеуровневые представления (Views)

| Название | Код | Описание |
|---|---|---|
| **Hotkeys View** | `viewHotkeys` | Основное представление — справочник горячих клавиш инструментов. Открывается при старте. |
| **My Tools View** | `viewMyTools` | Представление для управления личными инструментами: статусы, теги, заметки. |

Переключение между ними — клавиша `Tab` из левой панели.

---

## Глобальный макет (Hotkeys View)

```
┌──────────────────────────────────────────────────────────────┐
│  [Top Tabs: Hotkeys | My Tools]                              │
├──────────────────┬───────────────────────────────────────────┤
│                  │  [Header]                                  │
│   Left Panel     │  [Inner Tabs: Keys | Commands]            │
│  (Tool List)     ├───────────────────────────────────────────┤
│                  │                                            │
│                  │   Right Panel / Viewport                   │
│                  │   (Bindings / Commands / Search results)   │
│                  │                                            │
├──────────────────┴───────────────────────────────────────────┤
│  Help Bar                                                     │
└──────────────────────────────────────────────────────────────┘
```

---

## Панели (Panels)

### Left Panel — Левая панель
- Код: `renderLeft()`, `focusLeft`, константа `leftWidth = 22`
- Узкий вертикальный список всех загруженных инструментов.
- Каждая строка: `● ИмяИнструмента` (без числовых счётчиков).
- Индикатор обновления: `↑` после имени, если доступна новая версия.
- Активный инструмент обозначается кружком `●` (Selection Bar) — только при `focusLeft`.
- Граница: `PanelBorder` (неактивна) / `PanelBorderFocused` (активна, оранжевый).

### Right Panel — Правая панель
- Код: `renderRight()`, `focusRight`
- Занимает остаток ширины экрана.
- Содержит: Header + Inner Tabs (опционально) + Viewport.
- Граница: `PanelBorder` / `PanelBorderFocused`.

---

## Элементы Right Panel

### Header — Заголовок правой панели
- Код: `renderHeader()`
- Строка над контентом: название инструмента, версия, статус обновления, описание, ссылка на GitHub.
- Снизу отделён горизонтальным divider-ом цвета `ColorBorder`.
- При фокусе на Header элементе (`focusHeader`) — перед названием появляется `●` (`SelectionBarStyle`).
- Компоненты:
  - **Tool Name** — название (`TitleStyle`, оранжевый, жирный).
  - **Installed Version** — версия (`v1.2.3`, серый). Если инструмент не установлен — `not installed` (`MetaNoteStyle`, серый курсив).
  - **Update Badge** — `v1.24 -> v1.25` (жёлтый, жирный), если доступна новая версия.
  - **Version OK** — `✓` (зелёный), если версия актуальна.
  - **Tool Description** — краткое описание (`HeaderDescStyle`, серый курсив).
  - **GitHub Link** — `↗ owner/repo` (`GithubStyle`, серый).
  - **Repository Status** — `(active)` или `(archived)` после GitHub Link (`RepoStatusStyle`, серый курсив). Получается из GitHub API.

### Inner Tabs — Внутренние табы
- Код: `tabKeys = 0`, `tabCommands = 1`, `rightTab`
- Отображаются только если у инструмента есть группы команд (`CommandGroups`).
- Сверху отделены горизонтальным divider-ом цвета `ColorBorder`.
- **[Keys] Tab** — вкладка горячих клавиш (по умолчанию).
- **[Commands] Tab** — вкладка команд CLI/shell.
- Активная вкладка обёрнута в `[ ]` и выделена цветом (`TabActiveStyle`).

### Viewport — Область прокрутки
- Код: `viewport.Model` (библиотека `charmbracelet/bubbles/viewport`)
- Прокручиваемая область под заголовком в правой панели.
- В зависимости от активной вкладки отображает:
  - **Bindings** — список горячих клавиш (Keys Tab).
  - **Commands** — список CLI-команд (Commands Tab).
  - **Search Results** — результаты поиска (в режиме поиска).

---

## Содержимое Viewport

### Category — Категория
- Код: `Category`, `CategoryStyle`
- Заголовок группы биндингов или команд внутри инструмента (жирный, оранжево-персиковый).
- Пример: `Navigation`, `Editing`, `Git`.

### Binding Row — Строка биндинга
- Код: `Binding`, `renderTool()`
- Одна запись в списке горячих клавиш: `[Key] [Description]`.
- **Key** — горячая клавиша (`KeyStyle`, бежевый, фиксированная ширина 22).
- **Desc** — описание действия (`DescStyle`, светло-серый).
- Выделенная строка помечается `●` слева (`SelectionBarStyle`).

### Command Row — Строка команды
- Код: `Command`, `renderCommandsTab()`
- Одна запись в Commands Tab: `[cmd] [description]`.
- **Cmd** — команда (`CommandCmdStyle`, бежевый, жирный, ширина 30).
- **Desc** — описание (`CommandDescStyle`, светло-серый).
- Выделенная строка помечается `●`.

### Selection Bar — Индикатор выбора
- Код: `SelectionBarStyle`
- Символ `●` оранжевого цвета слева от активной строки в любом списке.
- В Left Panel отображается только при `focusLeft` — исчезает при переходе в Right Panel.

---

## Help Bar — Строка подсказок

- Код: `renderHelp()`, `HelpStyle`
- Полноширинная строка в самом низу экрана с округлой рамкой.
- Содержит контекстные подсказки клавиш в формате `[key] действие`.
- Меняется в зависимости от состояния:
  - Focus Tools (`focusTools`): `[/] search  [t] track  [u] untrack  [r] rename  [q] quit`.
  - Focus Brief (`focusBrief`, центральная карточка): `[o] open repo  [c] changelog  [s] status  [e] note  [t] tags  [q] quit` — действия над данными, которые карточка уже показывает; навигационные подсказки (scroll/help/back) намеренно убраны, сами клавиши работают.
  - Focus Help (`focusHelp`): `[↑↓] scroll  [h] --help  [m] man  [/] search  [←] back  [q] quit`.
  - Search Mode: поле ввода и выход из поиска.
  - Status Message: временное сообщение (например `no repo for <tool>`).

### Key Hint — Подсказка клавиши
- Код: `keyHint()`
- Отдельный элемент `[key]` внутри Help Bar (`SearchPromptStyle`, оранжевый).

---

## Top Tabs — Верхние табы

- Код: `TopTabActiveStyle` / `TopTabInactiveStyle`
- Расположены в верхней части Left Panel (в Hotkeys View) или вверху страницы (в My Tools View).
- Показывают текущее представление: `[Hotkeys]` и `My Tools` (или наоборот).
- Активная вкладка в `[ ]`, оранжевый; неактивная — серая.

---

## Оверлеи (Overlays / Popups)

### Command Popup — Попап команды
- Код: `renderPopup()`, `PopupStyle`, `showPopup`
- Центрированный оверлей поверх основного экрана.
- Открывается по `Enter` на команде в Commands Tab.
- Содержит: команду (`CommandCmdStyle`), описание (`CommandDescStyle`), подсказки `[y] copy [esc] close`.
- Граница: `PopupStyle` (оранжевая скруглённая рамка, padding 1×2).

### Changelog Popup — Попап чейнджлога
- Код: `renderChangelog()`, `showChangelog`
- Центрированный оверлей с информацией о последнем релизе инструмента на GitHub.
- Открывается по клавише `c` в Right Panel или в Header Focus (только если у инструмента задан `github`).
- Содержит: тег релиза, дата публикации, текст релиза (markdown стрипнут, перенос по ширине).
- Граница: оранжевая скруглённая рамка (`ColorPrimary`) — подсвечивается как активный оверлей.
- Прокручивается через `changelogViewport`.

---

## My Tools View

### My Tools List — Список инструментов
- Код: `renderMyToolsList()`
- Полноширинная панель со списком отслеживаемых инструментов.
- Каждая строка: `● <status-symbol> <status> <name> <tags>`.
- Внизу — **Stats Footer** со счётчиками по статусам.

### My Tools Detail — Детали инструмента
- Код: `renderMyToolsDetail()`
- Центрированный попап (`PopupStyle`) поверх My Tools.
- Открывается по `Enter` на инструменте в списке.
- Поля: название, статус, дата добавления (`Added`), теги (`Tags`), заметка (`Note`).
- Поля Tags и Note редактируются инлайн (через `textinput`).

### Stats Footer — Итоговая строка статистики
- Код: `countStatuses()`
- Строка внизу My Tools List: `N tools · N active · N trying · N forgotten · N archived`.
- Стиль: `MetaNoteStyle` (серый курсив).

### Status Symbol — Символ статуса
- Код: `loader.StatusSymbol`
- Однобуквенный/символьный индикатор статуса инструмента слева от строки.
- Цвет соответствует статусу (зелёный, жёлтый, серый, тёмно-серый).

---

## Статусы инструментов (My Tools)

| Статус | Цвет | Описание |
|---|---|---|
| `active` | Зелёный | Активно используется |
| `trying` | Жёлтый | В процессе освоения |
| `forgotten` | Серый | Не используется / забыт |
| `archived` | Тёмно-серый | В архиве |

---

## Search Mode — Режим поиска

- Код: `searching`, `textinput.Model` (поле `search`)
- Активируется клавишей `/`.
- Строка поиска отображается в Help Bar вместо подсказок.
- Результаты — в Viewport (grouped по инструментам, совпадения подсвечены `SearchMatchStyle`).
- Поиск по Key и Desc биндингов.
- Выход: `Esc`.

---

## Цветовая палитра

| Переменная | HEX | Применение |
|---|---|---|
| `ColorPrimary` | `#DA7756` | Акцентный оранжевый: рамки фокуса, заголовки, индикатор выбора |
| `ColorMuted` | `#AAAAAA` | Вторичный текст, неактивные элементы |
| `ColorBg` | `#0A0A0A` | Фон |
| `ColorBorder` | `#555555` | Рамки неактивных панелей |
| `ColorText` | `#E8E8E8` | Основной текст |
| `ColorCategory` | `#E8A87C` | Заголовки категорий |
| `ColorKey` | `#C8A97E` | Горячие клавиши, подсвеченные совпадения |
| `StatusColorActive` | `#6AAF6A` | Статус active |
| `StatusColorTrying` | `#E5A040` | Статус trying |
| `StatusColorForgotten` | `#AAAAAA` | Статус forgotten |
| `StatusColorArchived` | `#555555` | Статус archived |
