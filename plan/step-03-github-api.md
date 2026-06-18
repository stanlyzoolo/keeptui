# Шаг 3: Расширение GitHub API

## Цель
Получать из GitHub все данные для Tool Card:
- About (описание репозитория)
- Stars ★
- Latest Release (тег + дата)
- Languages (процентное соотношение)
- Changelog (уже работает, нужно только переиспользовать)

---

## Расширение `CacheEntry` в `internal/version/github.go`

```go
type CacheEntry struct {
    // Существующие поля:
    Latest      string    `json:"latest"`
    CheckedAt   time.Time `json:"checked_at"`
    Body        string    `json:"body,omitempty"`
    HtmlUrl     string    `json:"html_url,omitempty"`
    PublishedAt string    `json:"published_at,omitempty"`
    RepoStatus  string    `json:"repo_status,omitempty"`

    // НОВЫЕ поля:
    About     string         `json:"about,omitempty"`      // repo description
    Stars     int            `json:"stars,omitempty"`      // stargazers_count
    Languages map[string]int `json:"languages,omitempty"`  // bytes per language
}
```

---

## Расширение `fetchRepoInfo()`

Текущая функция возвращает только `(string, error)` — статус archived/active.  
Заменить возвращаемый тип:

```go
type RepoInfo struct {
    Status    string         // "active" | "archived"
    About     string         // description
    Stars     int            // stargazers_count
}

func fetchRepoInfo(repo string) (RepoInfo, error) {
    // GET /repos/{owner}/{repo}
    // Парсить поля: archived, description, stargazers_count
}
```

---

## Новая функция `fetchLanguages()`

```go
func fetchLanguages(repo string) (map[string]int, error) {
    // GET /repos/{owner}/{repo}/languages
    // Возвращает {"Go": 125000, "Shell": 4200, "Makefile": 800}
}
```

> Этот запрос делается **один раз** при первом fetch и кешируется в `CacheEntry.Languages`.  
> Языки меняются редко — TTL такой же, 24ч.

---

## Новый экспортируемый тип `RepoCard`

```go
// internal/version/github.go

type RepoCard struct {
    About       string
    Stars       int
    Languages   map[string]int  // raw bytes
    Latest      string          // release tag
    PublishedAt string
    HtmlUrl     string
    Body        string          // changelog
    RepoStatus  string
}

func GetRepoCard(githubField string) RepoCard {
    // Читает из кэша; если кэш свежий — возвращает сразу
    // Если устарел — запускает fetch (вызывается асинхронно из model)
}
```

---

## Вычисление процентов Languages

Делается в слое рендеринга, не в API-слое:

```go
func languagePercents(langs map[string]int) []LangRow {
    total := 0
    for _, b := range langs {
        total += b
    }
    // сортировать по убыванию bytes
    // для каждого: percent = bytes * 100 / total
    // оставить топ-5, остальное в "Other"
}
```

### Визуализация в правой панели (Tool Card):

```
Languages
  ██████████████░░░░  Go       87%
  ██░░░░░░░░░░░░░░░░  Shell     8%
  █░░░░░░░░░░░░░░░░░  Makefile  5%
```

Цветовая палитра для языков — фиксированная map `languageColor`:
```go
var languageColor = map[string]string{
    "Go":         "#00ADD8",
    "Rust":       "#DEA584",
    "Python":     "#3572A5",
    "TypeScript": "#3178C6",
    "Shell":      "#89E051",
    // fallback: ColorMuted (#AAAAAA)
}
```

---

## Асинхронная загрузка в `model.go`

Добавить новое сообщение:
```go
type repoCardMsg struct {
    toolName string
    card     version.RepoCard
    err      error
}
```

В `Init()` для каждого инструмента с непустым `GitHub` запускать `fetchRepoCardCmd()`:
```go
func fetchRepoCardCmd(t loader.Tool) tea.Cmd {
    return func() tea.Msg {
        card := version.GetRepoCard(t.GitHub)
        return repoCardMsg{toolName: t.Name, card: card}
    }
}
```

---

## Проверка после шага
```bash
# Запустить TUI, выбрать инструмент с GitHub — в правой панели должны появиться Stars и Languages
go run . 
```
