# Шаг 5: Переделка правой панели

## Цель
Правая панель делится на две горизонтальные половины без табов:
- **Левая половина** — Tool Card (данные из GitHub)
- **Правая половина** — вывод `--help` или `man`

---

## Макет

```
┌─ Right Panel ──────────────────────────────────────────────────────────┐
│ neovim  ● active                                                        │
│ ─────────────────────────────────────────────────────────────────────  │
│                          │                                              │
│  About                   │  Usage: nvim [options] [file ..]            │
│  Hyperextensible Vim-    │                                              │
│  based text editor       │  Options:                                   │
│                          │    --help        print this help            │
│  ─────────────────────   │    -v            print version info         │
│  ★  Stars    82.4k       │    -e            start in Ex mode           │
│  ⬆  Release  v0.10.4     │    -E            start in improved Ex mode  │
│     2024-11-14           │    -s            silent (batch) mode        │
│                          │    -d            diff mode                  │
│  ─────────────────────   │    -R            read-only mode             │
│  Languages               │    ...                                      │
│  ██████████  C    47%    │                                              │
│  ████░░░░░░  Lua  33%    │                                              │
│  ██░░░░░░░░  Vim  14%    │                                              │
│  █░░░░░░░░░  C++   6%    │                                              │
│                          │                                              │
│  ─────────────────────   │                                              │
│  Changelog  v0.10.4      │                                              │
│  # Highlights            │                                              │
│  - tree-sitter upgrade   │                                              │
│  ...                     │                                              │
└──────────────────────────┴─────────────────────────────────────────────┘
  [e] note  [t] tags  [s] status  [h] --help  [m] man  [o] open GitHub
```

---

## Разбивка ширины

```go
const leftWidth = 22  // левая панель (список инструментов)

// В renderRight():
rightTotal := m.width - leftWidth - 6
cardWidth  := rightTotal / 2        // Tool Card
helpWidth  := rightTotal - cardWidth // Help Output
```

---

## Новые viewport'ы в `Model`

```go
// Добавить в Model:
cardViewport viewport.Model  // прокрутка Tool Card
helpViewport viewport.Model  // прокрутка --help / man вывода
helpMode     int             // 0 = --help, 1 = man
helpLoading  bool
helpOutput   string          // кешированный вывод команды
```

---

## `renderCard()` — левая половина правой панели

Блоки разделены horizontal divider. Порядок:

```
About
<описание из GitHub>

───────────────────
★  Stars    82.4k
⬆  Release  v0.10.4
   2024-11-14

───────────────────
Languages
██████████  C    47%
████░░░░░░  Lua  33%

───────────────────
Note
<заметка пользователя>
Tags: editor, tui

───────────────────
Changelog  v0.10.4
<первые N строк body>
```

### Форматирование Stars
```go
func formatStars(n int) string {
    if n >= 1000 {
        return fmt.Sprintf("%.1fk", float64(n)/1000)
    }
    return strconv.Itoa(n)
}
```

### Language bar
```go
const barWidth = 10

func renderLangBar(pct int) string {
    filled := barWidth * pct / 100
    bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
    return lipgloss.NewStyle().Foreground(langColor).Render(bar)
}
```

---

## Header правой панели

Вместо старого `renderHeader()` — упрощённый:

```go
func (m Model) renderRightHeader() string {
    mt := m.selectedMeta()
    sym := loader.StatusSymbol[mt.Status]
    symStyled := ui.StatusStyle(mt.Status).Render(sym + "  " + string(mt.Status))
    return ui.TitleStyle.Render(mt.Name) + "  " + symStyled
}
```

---

## Инлайн-редактирование в Tool Card

При нажатии `e` (note) или `t` (tags) в строке Note/Tags появляется `textinput`:

```
Note
> изучаю LSP конфиги_   ← textinput активен
```

`Enter` — сохранить, `Esc` — отменить.

---

## Проверка после шага
```bash
go run .
# Выбрать инструмент с GitHub — видна Tool Card слева
# Stars, Languages, Changelog отображаются
# Правая половина пока пустая (заполняется в шаге 6)
```
