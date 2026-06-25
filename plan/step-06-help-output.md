# Шаг 6: Вывод `--help` и `man`

## Цель
Правая половина правой панели показывает вывод `<tool> --help` или `man <tool>`.  
Переключение клавишами `h` (--help) и `m` (man).

---

## Асинхронный запуск команды

### Новые сообщения Bubble Tea

```go
type helpOutputMsg struct {
    toolName string
    mode     int    // 0 = --help, 1 = man
    output   string
    err      error
}
```

### Tea.Cmd для запуска

```go
func fetchHelpCmd(name string, mode int) tea.Cmd {
    return func() tea.Msg {
        var out []byte
        var err error

        if mode == 0 {
            // --help
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            out, err = exec.CommandContext(ctx, name, "--help").CombinedOutput()
            if err != nil && len(out) == 0 {
                // попробовать -h
                out, err = exec.CommandContext(ctx, name, "-h").CombinedOutput()
            }
        } else {
            // man
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            // MANPAGER=cat убирает интерактивный pager
            cmd := exec.CommandContext(ctx, "man", name)
            cmd.Env = append(os.Environ(), "MANPAGER=cat", "MANWIDTH=80")
            out, err = cmd.Output()
        }

        return helpOutputMsg{
            toolName: name,
            mode:     mode,
            output:   stripANSI(string(out)),
            err:      err,
        }
    }
}
```

---

## Стрипание ANSI-кодов

```go
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
    return ansiRe.ReplaceAllString(s, "")
}
```

---

## Обработка в `Update()`

```go
case helpOutputMsg:
    if msg.toolName == m.selectedMeta().Name {
        m.helpLoading = false
        if msg.err != nil {
            if msg.mode == 0 {
                m.helpViewport.SetContent(ui.MetaNoteStyle.Render("--help not available"))
            } else {
                m.helpViewport.SetContent(ui.MetaNoteStyle.Render("man page not available"))
            }
        } else {
            m.helpViewport.SetContent(msg.output)
        }
        m.helpViewport.GotoTop()
    }
```

---

## Триггеры загрузки

1. **При смене инструмента** в левой панели — автоматически запускать `fetchHelpCmd(name, m.helpMode)`
2. **Клавиша `h`** — переключить на `--help`, запустить fetch если не кешировано
3. **Клавиша `m`** — переключить на `man`, запустить fetch если не кешировано

### Кеширование вывода

Хранить в `Model` map:
```go
helpCache map[string][2]string  // [toolName] → [helpOutput, manOutput]
```

Перед запуском проверять кеш — не запускать повторно если уже есть вывод.

---

## Состояние загрузки

Пока вывод загружается — показывать в правой половине:
```
  loading...
```

---

## Подсветка синтаксиса (опционально, v2)

Для `--help` можно выделить флаги цветом:
- Строки вида `  -v, --verbose   ...` — флаг выделить `ColorKey` (`#C8A97E`)
- Команды/секции типа `Usage:`, `Options:` — выделить `ColorCategory`

Простая реализация через regexp:
```go
var flagRe  = regexp.MustCompile(`^\s+(-\w|--\w[\w-]*)`)
var sectionRe = regexp.MustCompile(`^[A-Z][A-Z ]+:`)
```

---

## Поиск внутри вывода `--help` / `man`

Активируется клавишей `/` когда фокус на правой половине (Help Output).  
Работает независимо от поиска в левой панели.

### Поведение

```
┌─ Help Output ────────────────────────────────────┐
│  Usage: bat [OPTIONS] [FILE]...                  │
│                                                  │
│  Options:                                        │
│  ██-l, --language <language>██                   │  ← подсветка совпадения
│      Set the language for syntax highlighting    │
│  -H, --highlight-line <N:M>                      │
│      Highlight lines N through M                 │
│  ...                                             │
├──────────────────────────────────────────────────┤
│ / --lang_          [n] next  [N] prev  [Esc] exit│
└──────────────────────────────────────────────────┘
```

### Состояние в `Model`

```go
// Добавить в Model:
helpSearching  bool
helpSearch     textinput.Model  // отдельный input для поиска в help
helpMatches    []int            // индексы строк-совпадений
helpMatchIdx   int              // текущее совпадение
```

### Алгоритм

1. Нажать `/` при фокусе на правой половине → `helpSearching = true`, активировать `helpSearch`
2. При каждом изменении query — искать по строкам `helpOutput`:
   ```go
   func findMatches(text, query string) []int {
       lines := strings.Split(text, "\n")
       q := strings.ToLower(query)
       var result []int
       for i, line := range lines {
           if strings.Contains(strings.ToLower(line), q) {
               result = append(result, i)
           }
       }
       return result
   }
   ```
3. Проскроллить `helpViewport` к первому совпадению (`helpMatchIdx = 0`)
4. `n` → следующее совпадение, `N` → предыдущее, `Esc` → выйти из поиска

### Подсветка совпадений в рендеринге

При `helpSearching && query != ""` заменить рендеринг строк viewport:
- Совпадающий фрагмент выделить `SearchMatchStyle` (`ColorKey` + bold)
- Строка с активным совпадением (`helpMatchIdx`) — дополнительно инвертировать фон

Используем `strings.Replace` с `lipgloss` wrap для подсветки:
```go
func highlightMatch(line, query string) string {
    q := strings.ToLower(query)
    idx := strings.Index(strings.ToLower(line), q)
    if idx < 0 {
        return line
    }
    before := line[:idx]
    match  := line[idx : idx+len(query)]
    after  := line[idx+len(query):]
    return before + ui.SearchMatchStyle.Render(match) + after
}
```

### Навигация по совпадениям

`helpViewport.SetYOffset(helpMatches[helpMatchIdx])` — прокрутить к нужной строке.

---

## Help Bar

```
[j/k] scroll  [h] --help  [m] man  [/] search  [e] note  [s] status  [o] GitHub  [q] quit
```

В режиме поиска по help output:
```
/ --lang_          [n] next  [N] prev  [Esc] exit search       3 matches
```

При `helpLoading`:
```
[h] --help  [m] man  loading...
```

---

## Fallback-поведение

| Ситуация | Отображение |
|---|---|
| Инструмент не установлен | `"neovim: command not found"` |
| `--help` выходит с кодом ≠ 0, но есть вывод | показать вывод (многие инструменты так делают) |
| `man` не найден (macOS без man pages) | `"man page not available"` |
| Таймаут (>5с) | `"command timed out"` |

---

## Проверка после шага
```bash
go run .
# Выбрать bat — справа должен появиться вывод bat --help
# Нажать m — появляется man bat (или сообщение об отсутствии)
# Нажать h — вернуться к --help
# Переключиться на другой инструмент — вывод обновляется
```
