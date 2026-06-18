##### Центральная модель `Tool` включает в себя общую информацию, которую получаем, преимущественно, из github.com:

* Description (e.g. brief from About section from github repository)
* Source (e.g. github.com/stanlyzoolo/keepkeys)
* Status (e.g. active, archived and etc. NEED RESEARCH)
* Version (e.g. v0.12.4)
* Changelog (if available)

---

## Что можно получить из GitHub API по репозиторию

### Уже реализовано (через `/releases/latest`)
| Поле | Откуда | Используется |
|---|---|---|
| `tag_name` | releases/latest | версия Latest в хедере |
| `body` | releases/latest | changelog overlay (`[c]`) |
| `html_url` | releases/latest | ссылка в changelog |
| `published_at` | releases/latest | дата релиза в changelog |

---

### Доступно из GitHub API — стоит добавить

#### `GET /repos/{owner}/{repo}` — основной эндпоинт репозитория
| Поле | Тип | Применение в keepkeys |
|---|---|---|
| `description` | string | описание инструмента (About) — автозаполнение при `keys track` |
| `stargazers_count` | int | популярность — можно показать в карточке инструмента |
| `forks_count` | int | активность сообщества |
| `open_issues_count` | int | сигнал здоровья проекта |
| `topics` | []string | теги из GitHub — автозаполнение поля `Tags` в ToolMeta |
| `archived` | bool | маркер "заброшен" → автоматически ставить статус `archived` |
| `pushed_at` | string (ISO8601) | дата последнего коммита — сигнал заброшенности |
| `license.spdx_id` | string | лицензия (MIT, Apache-2.0) |
| `language` | string | основной язык — полезно как фильтр/тег |
| `homepage` | string | сайт / документация инструмента |
| `default_branch` | string | нужен для построения raw-ссылок |

#### `GET /repos/{owner}/{repo}/releases` — список релизов
| Поле | Применение |
|---|---|
| весь список `tag_name` + `published_at` | история версий (сколько релизов, как часто выходят) |
| `prerelease`, `draft` | фильтрация — не показывать пре-релизы как Latest |

#### `GET /repos/{owner}/{repo}/contributors` (или `stats/commit_activity`)
| Поле | Применение |
|---|---|
| количество контрибьюторов | дополнительный сигнал активности |
| commit activity за год | график активности (не приоритет) |

---

### Производные метрики (вычисляем сами)

| Метрика | Как считаем | Зачем |
|---|---|---|
| **Freshness** | `now - pushed_at` | показываем "последнее обновление N дней назад" |
| **Release cadence** | разница дат между последними релизами | "выходит раз в ~30 дней" |
| **Health score** | archived=false + pushed < 180 дней + open_issues < threshold | авто-статус "healthy / stale / dead" |
| **Auto-status suggestion** | archived=true → предлагать `archived`; pushed > 365 дней → предлагать `forgotten` | подсказка при `keys check` |

---

### Что требует отдельного исследования

- **Changelog из CHANGELOG.md**: некоторые проекты не используют GitHub Releases, а ведут файл `CHANGELOG.md` в корне. Можно получить через `GET /repos/{owner}/{repo}/contents/CHANGELOG.md` (base64 decode) — нужно как fallback.
- **Статус инструмента**: сопоставление `archived` + `pushed_at` + `open_issues_count` → enum `active / stale / archived`. Формула требует тюнинга.
- **Альтернативные источники версий**: не все проекты делают GitHub Releases (например, только теги). Эндпоинт `/releases/latest` вернёт 404 — нужен fallback на `/tags`.

---

### Итоговое расширение модели `Tool`

```go
// Поля, которые стоит добавить в struct Tool (loader/loader.go)
// и кешировать рядом с CacheEntry (version/github.go):

type RepoInfo struct {
    Description   string   // About из репозитория
    Stars         int      // stargazers_count
    Topics        []string // теги из GitHub
    Archived      bool     // archived
    PushedAt      string   // дата последнего push
    License       string   // spdx_id
    Language      string   // основной язык
    Homepage      string   // ссылка на сайт / docs
    OpenIssues    int      // open_issues_count
}
```

Всё это получается одним запросом `GET /repos/{owner}/{repo}`, который можно кешировать с тем же TTL что и релизы (24h), либо с отдельным TTL (например, 12h).
