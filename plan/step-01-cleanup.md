# Шаг 1: Удаление старой архитектуры

## Цель
Убрать весь код, связанный с хранением горячих клавиш, YAML-конфигами и tldr.

---

## Файлы для полного удаления

| Файл | Причина |
|---|---|
| `internal/cmd/fetch.go` | Загрузка из tldr больше не нужна |
| `internal/cmd/new.go` | Создание YAML-конфигов больше не нужно |
| `internal/cmd/edit.go` | Редактирование YAML больше не нужно |
| `internal/cmd/import.go` | Импорт YAML больше не нужен |
| `internal/cmd/validate.go` | Валидация YAML больше не нужна |
| `internal/cmd/check.go` | Проверка версий переезжает в GitHub API шага 3 |
| `internal/loader/validate.go` | Валидация схемы Tool |
| `internal/tldr/cache.go` | tldr полностью удаляется |
| `internal/tldr/parse.go` | tldr полностью удаляется |
| `internal/loader/data/` | Все embedded YAML-конфиги инструментов |

---

## Файлы для модификации

### `internal/loader/loader.go`
Удалить типы и поля:
```go
// УДАЛИТЬ эти типы:
type Binding struct { ... }
type Category struct { ... }
type Command struct { ... }
type CommandGroup struct { ... }

// УДАЛИТЬ поля из Tool:
Categories    []Category     `yaml:"categories"`
CommandGroups []CommandGroup `yaml:"command_groups,omitempty"`

// УДАЛИТЬ директиву embed:
//go:embed data/tools
var Embedded embed.FS

// УДАЛИТЬ функцию Load() полностью — заменяется в шаге 2
```

Оставить только:
```go
type Tool struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    GitHub      string `yaml:"github"`
    VersionCmd  string `yaml:"version_cmd"`
    Source      string `yaml:"-"`
}
```

### `internal/cmd/helpers.go`
Удалить функции:
- `userToolsDir()`
- `userToolPath()`
- `openEditor()`
- `editAndValidate()`
- `confirmOverwrite()`

Оставить:
- `validateToolName()`
- `confirm()`

### `main.go`
Удалить из `runCommand()` case-блоки: `new`, `import`, `edit`, `validate`, `fetch`, `check`.  
Удалить соответствующие строки из `helpText`.  
Удалить импорт `loader.Embedded` (больше не используется).

### `internal/model/model.go`
Удалить все обращения к:
- `t.Categories`
- `t.CommandGroups`
- `tabKeys`, `tabCommands`, `rightTab`
- `renderTool()`, `renderCommandsTab()`, `renderSearchResults()`
- `selectedBinding`, `selectedCommand`
- `showPopup`, `popupCommand`
- `renderPopup()`, `renderChangelog()` (changelog переедет в шаг 5)

---

## Проверка после шага
```bash
go build .      # должен компилироваться без ошибок
go vet ./...    # никаких предупреждений
```
