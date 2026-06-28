# keys
 
Terminal TUI для просмотра горячих клавиш и команд CLI-инструментов. Открываешь — находишь нужный шорткат — копируешь в буфер — закрываешь.

```
┌─[Hotkeys]  My Tools─────────────────────────────────────────────────────────┐
│ ● neovim  142 │ neovim  v0.10.1 ✓  Hyperextensible Vim-based text editor    │
│   tmux     58 │ [Keys]  Commands                                             │
│   yazi     44 │                                                              │
│   helix    96 │ Navigation                                                   │
│   fzf      21 │   h / l        move left / right                            │
│   kubectl  33 │ ● j / k        move down / up                               │
│   ...         │   w / b        next / previous word                         │
│               │   gg / G       file start / end                             │
└───────────────┴─────────────────────────────────────────────────────────────┘
  [/] search  [t] track  [u] untrack  [r] rename  [q] quit
```

## Возможности

- **Два режима**: Hotkeys (шорткаты по категориям) + Commands (примеры команд из tldr)
- **Поиск** — `/` ищет сразу по всем инструментам и всем биндингам с подсветкой совпадений
- **Копирование** — `y` копирует выбранный шорткат или команду в буфер обмена
- **Версии** — показывает установленную версию и уведомляет об обновлениях с GitHub
- **My Tools** — трекер инструментов со статусами, тегами и заметками
- **tldr-интеграция** — `keys fetch <tool>` загружает примеры команд из tldr-pages
- **Пользовательские конфиги** — `~/.config/keys/tools/<tool>/config.yaml` перекрывает встроенный
- **Мышь** — поддержка скролла и клика

## Встроенные инструменты

atuin · bat · delta · docker · fzf · gh · helix · kubectl · micro · neovim · ripgrep · tmux · yazi · zellij

## Установка

**Из исходников** (требуется Go 1.25+):

```bash
git clone https://github.com/lepeshko/keys
cd keys
go install .
```

Бинарник попадает в `~/go/bin/keys`. Убедись, что `~/go/bin` есть в `PATH`.

## Команды

```
keys                          открыть TUI
keys <tool>                   открыть TUI сразу на инструменте (напр. keys tmux)
keys -s <query>               открыть TUI с предзаполненным поиском
keys -h, --help               помощь
```

### Конфиги

```
keys new <tool>               создать ~/.config/keys/tools/<tool>/config.yaml и открыть в $EDITOR
keys edit <tool>              открыть пользовательский конфиг в $EDITOR
keys edit --builtin <tool>    скопировать встроенный конфиг в ~/.config/keys/ и открыть в $EDITOR
keys import <url|path>        импортировать YAML (с валидацией перед сохранением)
keys validate <path>          проверить YAML-файл без импорта
```

### Список и фильтрация

```
keys list                     все инструменты
keys list --active            только со статусом active
keys list --trying            только trying
keys list --forgotten         только forgotten
keys list --archived          только archived
keys list --tag <name>        фильтр по тегу
```

### Версии

```
keys check <tool>             установленная и актуальная версия
keys check --all              все инструменты
keys check --outdated         только инструменты с доступными обновлениями
```

### Загрузка команд из tldr

```
keys fetch <tool>             загрузить примеры из tldr-pages и добавить в command_groups
```

Команда запрашивает подтверждение перед записью. Если пользовательского конфига нет — копирует встроенный.

### Трекер инструментов

Добавление и удаление инструментов из трекера выполняется прямо в TUI (левая панель со списком):

| Клавиша | Действие |
|---------|----------|
| `t` | track — добавить по GitHub-ссылке или короткому имени |
| `u` | untrack — удалить (с подтверждением `enter` / `esc`) |
| `r` | rename — поправить имя бинарника, если оно отличается от имени репозитория (напр. `claude-code` → `claude`) |

При вводе GitHub-ссылки (`https://github.com/owner/repo`, с `.git`, без схемы или в SSH-форме `git@github.com:owner/repo.git`) `keys` подставит короткое имя инструмента (`repo`) в `name` и нормализованный `github.com/owner/repo` в поле `github`. Новый инструмент получает статус `trying`.

Статус и заметку можно менять из CLI:

```
keys status <tool> active|trying|forgotten|archived   изменить статус
keys note <tool> "текст"                       обновить заметку
```

Статусы: `active` (●) · `trying` (○) · `forgotten` (~) · `archived` (✕)

## Навигация в TUI

### Hotkeys view

| Клавиша | Действие |
|---------|----------|
| `j / k`, `↓ / ↑` | навигация по инструментам / шорткатам |
| `→ / l` | перевести фокус в правую панель |
| `← / h`, `esc` | вернуть фокус в левую панель |
| `tab` | переключить вкладку Keys ↔ Commands |
| `/` | режим поиска |
| `esc` (в поиске) | выйти из поиска |
| `y` | копировать шорткат / команду в буфер |
| `t` | track — добавить инструмент (фокус на списке) |
| `u` | untrack — удалить инструмент (фокус на списке) |
| `r` | rename — переименовать инструмент (фокус на списке) |
| `enter` | детали команды (вкладка Commands) |
| `g / G` | перейти в начало / конец |
| `q`, `ctrl+c` | выход |

## Формат конфига

`~/.config/keys/tools/<tool>/config.yaml` — перекрывает встроенный конфиг с тем же именем.

```yaml
name: mytool
description: Краткое описание инструмента
github: github.com/owner/mytool        # без https://
version_cmd: mytool --version          # опционально; по умолчанию <name> --version

categories:
  - name: Navigation
    bindings:
      - key: "ctrl+n"
        desc: next item
      - key: "ctrl+p"
        desc: previous item

command_groups:                        # опционально; заполняется через keys fetch
  - name: Examples
    commands:
      - cmd: mytool init --force
        desc: initialize with overwrite
```

**Обязательные поля:** `name`, хотя бы одно из `categories` или `command_groups`.  
`name` не должен содержать `/`, `\`, `..`.

## Хранение данных

| Что | Где |
|-----|-----|
| Встроенные конфиги | встроены в бинарник (embed) |
| Пользовательские конфиги | `~/.config/keys/tools/<tool>/config.yaml` |
| Метаданные трекера | `~/.config/keys/meta.yaml` |

## Стек

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI-фреймворк
- [Bubbles](https://github.com/charmbracelet/bubbles) — текстовый ввод, viewport
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — стилизация
- [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — парсинг конфигов

## Вклад в проект

Новые конфиги инструментов приветствуются. Добавь YAML в `internal/loader/data/tools/<toolname>/config.yaml` и открой pull request.

## Лицензия

MIT
