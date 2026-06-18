# Шаг 4: Переделка левой панели

## Цель
Убрать Top Tabs (Hotkeys / My Tools). Остаётся один вид — Registry.  
Левая панель показывает только tracked tools из `meta.yaml`.

---

## Что убрать

- `viewHotkeys` / `viewMyTools` — два view-режима → один
- Top Tabs (`[Hotkeys]  My Tools`) из `renderLeft()`
- `renderMyToolsList()` и `renderMyToolsDetail()` — заменяются новым layout
- Переключение по `Tab` между видами

---

## Новый `renderLeft()`

```
┌─────────────────────┐
│ Filter: all      [f]│
│                     │
│ ● neovim   ● active │
│   bat      ○ trying │
│   lazygit  ~ forgot │
│   kubectl  ● active │
│                     │
│ 4 tools             │
└─────────────────────┘
```

Каждая строка: `<selection> <name>  <status-symbol>`

```go
func (m Model) renderLeft() string {
    var sb strings.Builder

    filterLabel := "all"
    if m.metaFilter != "" {
        filterLabel = string(m.metaFilter)
    }
    sb.WriteString(ui.MetaNoteStyle.Render("Filter: "+filterLabel) + "\n\n")

    for i, mt := range m.filteredMeta() {
        sym := loader.StatusSymbol[mt.Status]
        symStyled := ui.StatusStyle(mt.Status).Render(sym)
        name := mt.Name
        if len(name) > leftWidth-5 {
            name = name[:leftWidth-5]
        }
        isSelected := i == m.metaSelected && m.focus == focusLeft
        if isSelected {
            sb.WriteString(ui.SelectionBarStyle.Render("●") + "  " + name + "  " + symStyled + "\n")
        } else {
            sb.WriteString(ui.ToolNormalStyle.Render("   "+name) + "  " + symStyled + "\n")
        }
    }

    // footer
    sb.WriteString("\n")
    sb.WriteString(ui.MetaNoteStyle.Render(fmt.Sprintf("  %d tools", len(m.meta))) + "\n")

    panelStyle := ui.PanelBorder
    if m.focus == focusLeft {
        panelStyle = ui.PanelBorderFocused
    }
    return panelStyle.Width(leftWidth).Height(max(m.height-7, 1)).Render(sb.String())
}
```

---

## Навигация

| Клавиша | Действие |
|---|---|
| `j / k` | вверх/вниз по списку |
| `→` | перевести фокус в правую панель |
| `f` | циклический фильтр по статусу |
| `1/2/3/4` | фильтр: active / trying / forgotten / archived |
| `a` | сбросить фильтр |
| `s` | сменить статус выбранного инструмента |
| `/` | режим поиска по именам инструментов |
| `Esc` | выйти из поиска |
| `q` / `Ctrl+C` | выход |

---

## Поиск в левой панели

Активируется клавишей `/` — появляется строка поиска внизу левой панели (или в Help Bar).  
Фильтрует список инструментов по имени в реальном времени.

```
┌─────────────────────┐
│ Filter: all         │
│                     │
│ ● bat      ○ trying │  ← единственное совпадение для "ba"
│                     │
│ 1 of 4 tools        │
├─────────────────────┤
│ / ba_               │  ← строка поиска
└─────────────────────┘
```

### Поведение правой панели при поиске

При вводе в строку поиска правая панель **автоматически** переключается на первый совпавший инструмент:
- Tool Card загружается для первого результата
- Help Output (`--help`) запускается для первого результата
- Навигация `j/k` по отфильтрованному списку обновляет правую панель

### Реализация в `Model`

Переиспользовать существующие поля:
```go
search    textinput.Model  // уже есть в model.go
searching bool             // уже есть в model.go
```

Метод `filteredMeta()` расширить — при `searching == true` фильтровать по `strings.Contains(name, query)`:
```go
func (m Model) filteredMeta() []loader.ToolMeta {
    base := m.meta
    if m.metaFilter != "" {
        // фильтр по статусу (уже есть)
    }
    if m.searching {
        query := strings.ToLower(m.search.Value())
        filtered := base[:0]
        for _, mt := range base {
            if strings.Contains(strings.ToLower(mt.Name), query) {
                filtered = append(filtered, mt)
            }
        }
        base = filtered
    }
    return base
}
```

При входе в поиск сбросить `metaSelected = 0` и запустить загрузку Tool Card + Help для первого результата.

---

## Удалить из `Model`

```go
// Удалить:
view        viewMode   // больше не нужно — один view
metaDetail  bool       // попап деталей заменяется правой панелью
editingNote bool       // редактирование переедет в правую панель
editingTags bool       // то же
```

---

## Пустое состояние

Если `meta` пустой:
```
  No tools tracked.
  Add one: keys track <tool> --github github.com/owner/repo
```

---

## Проверка после шага
```bash
go run .
# Открыть TUI — должна быть только левая панель со списком tracked tools
# Tab больше не переключает виды
# f / 1-4 фильтруют список
```
