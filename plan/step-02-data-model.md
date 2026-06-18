# Шаг 2: Модель данных Registry

## Цель
`ToolMeta` становится единственным источником правды об инструменте.  
`Tool` (старая структура) либо удаляется, либо генерируется из `ToolMeta`.

---

## Изменения в `internal/loader/meta.go`

Добавить поля в `ToolMeta`:
```go
type ToolMeta struct {
    Name      string   `yaml:"name"`
    GitHub    string   `yaml:"github,omitempty"`   // НОВОЕ: github.com/owner/repo
    Status    Status   `yaml:"status"`
    Added     string   `yaml:"added"`
    Tags      []string `yaml:"tags,omitempty"`
    Note      string   `yaml:"note,omitempty"`
}
```

---

## Новая функция `loader.Load()`

Теперь `Load()` строит список инструментов из `meta.yaml`, а не из embedded YAML:
```go
func Load() ([]Tool, error) {
    meta, err := LoadMeta()
    if err != nil {
        return nil, err
    }
    tools := make([]Tool, 0, len(meta))
    for _, m := range meta {
        tools = append(tools, Tool{
            Name:   m.Name,
            GitHub: m.GitHub,
            Source: "registry",
        })
    }
    return tools, nil
}
```

> Поле `Description` будет приходить из GitHub API (About) и кешироваться — не хранится в meta.yaml.

---

## Изменения в `internal/cmd/track.go`

Добавить флаг `--github`:
```
keys track neovim --github github.com/neovim/neovim --status trying --tags tui,editor
```

Парсинг аргументов в `RunTrack()`:
```go
func RunTrack(args []string) error {
    if len(args) == 0 {
        return fmt.Errorf("usage: keys track <tool> [--github <repo>] [--status ...] ...")
    }
    name := args[0]
    // parse --github, --status, --tags, --note из args[1:]
    // ...
    entry := loader.ToolMeta{
        Name:   name,
        GitHub: githubFlag,  // может быть пустым
        Status: statusFlag,
        Tags:   tags,
        Note:   noteFlag,
        Added:  loader.TodayDate(),
    }
    // ...
}
```

---

## Обновление helpText в `main.go`

```
keys track <tool> [--github <repo>] [--status trying] [--tags a,b] [--note "..."]
```

---

## Обновление TUI: `model.New()`

Сигнатура меняется — `tools []loader.Tool` строится внутри из `meta`:
```go
// main.go
meta, _ := loader.LoadMeta()
tools := loader.MetaToTools(meta)  // новая вспомогательная функция

model.New(tools, meta, model.Options{...})
```

Либо `model.New` принимает только `meta` и сам строит tools — на выбор.

---

## Проверка после шага
```bash
keys track bat --github github.com/sharkdp/bat --status trying
keys list
# bat должен появиться в списке

go build .
go vet ./...
```
