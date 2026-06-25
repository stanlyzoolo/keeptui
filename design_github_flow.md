# UI/UX: GitHub refresh flow и карточка инструмента

## Контекст

keepkeys — TUI на Bubble Tea. Уже есть:
- Changelog overlay (`[c]`) через `ui.PlaceOverlay`
- Command popup (`enter` в Commands tab)
- My Tools detail view (центрированный блок)

Задача: добавить **карточку инструмента** (Tool Card overlay) с данными из GitHub и возможностью **рефрешить их из UI**.

---

## 1. Tool Card Overlay — новый тип оверлея

### Концепция

Нажатие `i` (info) на любом инструменте в левой панели или в правой панели (hotkeys view) открывает оверлей с полной информацией об инструменте.

```
┌─────────────────────────────────────────────────────┐
│  ripgrep  v14.1.1 ✓                         ↗ github│
│  ─────────────────────────────────────────────────  │
│  Fast search tool for source code                   │
│                                                     │
│  ★ 49 821    ⑂ 1 203    ⚠ 142 issues               │
│  Language: Rust    License: Unlicense               │
│  Last push: 12 days ago    Released: 2024-09-01     │
│  Topics: search, grep, rust, cli                    │
│                                                     │
│  Homepage: https://github.com/BurntSushi/ripgrep    │
│                                                     │
│  ─────────────────────────────────────────────────  │
│  [r] refresh from GitHub   [o] open   [esc] close   │
└─────────────────────────────────────────────────────┘
```

### Ключи

| Клавиша | Действие |
|---|---|
| `i` | открыть Tool Card overlay |
| `r` (внутри overlay) | force-refresh данных из GitHub (bypass cache) |
| `o` (внутри overlay) | открыть репозиторий в браузере |
| `c` (внутри overlay) | переключиться в Changelog overlay |
| `esc` / `q` | закрыть overlay |

### Состояния overlay

1. **Loading** — показываем спиннер или "Fetching from GitHub..."
2. **Loaded** — отображаем все поля
3. **Stale** — данные из кеша, показываем "Cached: N hours ago  [r] refresh"
4. **Error** — "Failed to fetch. Cached data shown." + дата кеша
5. **No GitHub** — "No GitHub URL configured for this tool"

---

## 2. Refresh из списка (без открытия overlay)

В левой панели (список инструментов) добавить хоткей `R` (Shift+R) — refresh версии текущего выбранного инструмента прямо в списке, без открытия оверлея. После обновления мигает индикатор `↑` или `✓` рядом с именем.

---

## 3. Интеграция с My Tools

В **My Tools detail view** (уже существует в `renderMyToolsDetail`) добавить секцию с GitHub-данными:

```
Tags:    [e] edit tags              ← уже есть
Note:    [e] edit note              ← уже есть
──────────────────────────────────
GitHub:
  Stars: 49 821   Issues: 142
  Last push: 12 days ago
  Health: ● active                  ← вычисляем из pushed_at + archived
  [r] refresh   [i] full card
```

Клавиша `r` в detail view рефрешит GitHub-данные для этого инструмента.

---

## 4. Авто-предложение статуса

Когда `keys check` или при открытии Tool Card: если `archived=true` или `pushed_at > 365 дней` — показываем баннер:

```
⚠ This repo hasn't been updated in 14 months.
  Suggest status: forgotten   [s] apply   [esc] skip
```

Баннер рендерится внутри Tool Card overlay как отдельная строка с жёлтым цветом (`UpdateAvailableStyle`).

---

## 5. Технические решения

### Новый тип сообщения

```go
type repoInfoMsg struct {
    toolName string
    info     version.RepoInfo
    cached   bool
    cachedAt time.Time
    err      error
}
```

### Новое состояние модели

```go
// в struct Model добавить:
showToolCard      bool
toolCardLoading   bool
toolCardToolName  string
toolCardInfo      version.RepoInfo
toolCardCached    bool
toolCardCachedAt  time.Time
toolCardErr       error
```

### Рендеринг

`renderToolCard()` → возвращает строку, рендерится через `ui.PlaceOverlay` (аналогично `renderChangelog()`).

### Fetch команда

```go
func fetchRepoInfoCmd(githubField, toolName string) tea.Cmd {
    return func() tea.Msg {
        info, cached, cachedAt, err := version.GetRepoInfo(githubField)
        return repoInfoMsg{toolName: toolName, info: info, cached: cached, cachedAt: cachedAt, err: err}
    }
}
```

---

## 6. Порядок реализации

1. Расширить `version.CacheEntry` полем `RepoInfo` и добавить `GetRepoInfo` / `FetchRepoInfo` в `github.go`
2. Добавить `repoInfoMsg` и состояние overlay в `Model`
3. Реализовать `renderToolCard()` в `model.go`
4. Подключить клавишу `i` в hotkeys view и my tools view
5. Добавить клавишу `r` внутри overlay (force-refresh)
6. Добавить баннер авто-предложения статуса
7. Расширить My Tools detail view секцией GitHub-данных
