# keeptui

A terminal TUI tracker for CLI tools: a list of tracked tools, a card with repository
data, versions and notes, the rendered repository README plus built-in `--help` / `man`
viewing, and updating outdated tools right from the interface. Pure TUI — no
subcommands; the only flags are `--version` and `--help`.

![keeptui — three-panel overview: tracker list, tool card, docs viewer (README / --help / man), live search and the hotkeys overlay](demo/hero.gif)

## Features

- **Three panels**: tools (the tracker list), brief (the tool card), docs (README / `--help` / `man`)
- **README first** — panel `[3]` opens on the repository README, rendered right in the terminal; `h` / `m` / `r` switch between `--help`, `man` and the README. A tool that is tracked but not installed still has a full panel — exactly the case where docs matter most
- **Tool card** — repository, stars, languages, installed and latest version with release date, status, note and tags
- **Versions** — the installed version is detected locally, the latest is fetched from GitHub; an outdated install is marked with `↑` in the list and on the card, and tools with an available update are grouped at the top of the list
- **In-TUI updates** — `u` on the card detects the package manager (brew / go / cargo / pipx / npm) or uses `update_cmd` from `meta.yaml`, shows the command for confirmation and streams its output into panel `[3]` in real time
- **Help navigation** — in `--help` / `man` mode `j` / `k` walk through flags and subcommands with the current entry highlighted; `/` searches the text
- **List search** — `/` filters by name and tags with match highlighting and an `N/M` counter
- **Tracker** — add by GitHub URL, statuses, tags and notes, all inside the TUI
- **GitHub API gauge** — an API quota usage indicator in the status bar, token management via `L`
- **Mouse** — scrolling and clicking on panels

## Installation

Homebrew (macOS / Linux):

```bash
brew install stanlyzoolo/apps/keeptui
```

Or tap once and install by name:

```bash
brew tap stanlyzoolo/apps
brew install keeptui
```

Upgrade later with `brew upgrade keeptui`.

From source (requires Go 1.25+):

```bash
git clone https://github.com/stanlyzoolo/keeptui
cd keeptui
go install .
```

The binary lands in `~/go/bin/keeptui`. Make sure `~/go/bin` is on your `PATH`.

## Usage

Run `keeptui` — the three-panel interface opens. Focus moves with `←` / `→` or the
digits `1` / `2` / `3` (each panel's number is written in its title). Press `?` at any
time for the hotkeys overlay — every keybinding, grouped by panel.

### Panel `[1] Tools`

| Key | Action |
|-----|--------|
| `j / k`, `↑ / ↓` | navigate the list (wraps around the edges) |
| `PgUp / PgDn`, `ctrl+f / ctrl+b` | page the selection up / down |
| `ctrl+d / ctrl+u` | move the selection half a page down / up |
| `g / G` | jump to the first / last tool |
| `t` | track — add a tool by GitHub URL or short name |
| `u` | untrack — remove (with confirmation) |
| `r` | rename — fix the binary name when it differs from the repo name (e.g. `claude-code` → `claude`) |
| `/` | search by name and tags: the matched substring is highlighted, tag-only matches show the tag dimmed, the status bar shows an `N/M` counter; `↑` / `↓` move through matches, `enter` opens the card, `esc` cancels and restores the previous selection |
| `L` | GitHub API status — limits and token (see below) |
| `?` | hotkeys overlay — every keybinding, grouped by panel |
| `esc`, `q`, `ctrl+c` | quit |

When you enter a GitHub URL (`https://github.com/owner/repo`, with `.git`, without a
scheme, or in SSH form `git@github.com:owner/repo.git`), `keeptui` puts the short tool
name into `name` and the normalized `github.com/owner/repo` into the `github` field.
A new tool gets the `trying` status.

The selected row carries the `⏺` marker, which stays visible (dimmed) while another
panel is focused. Tools with an available update are marked `↑` and gathered at the
top of the list; the order in `meta.yaml` is never changed.

### Panel `[2] Brief`

| Key | Action |
|-----|--------|
| `o` | open the repository in the browser |
| `c` | open the changelog / releases page in the browser |
| `u` | update the tool (available when marked `↑`); `enter` runs the shown command, `esc` cancels |
| `r` | force-refresh the tool's data (card, changelog, README, installed version), bypassing the cache |
| `s` | cycle the status (`active → trying → inactive → active`) |
| `e` | edit the note |
| `t` | edit the tags |
| `j / k`, `↑ / ↓` | scroll the card (3 lines) |
| `ctrl+d / ctrl+u`, `ctrl+f / ctrl+b`, `PgUp / PgDn`, `space`, `g / G` | half-page / full-page scroll, top / bottom |
| `?` | hotkeys overlay |

Statuses: `active` (●) · `trying` (○) · `inactive` (✕) — shown on the card.
Legacy `forgotten` / `archived` values from `meta.yaml` are automatically read as
`inactive`.

### Panel `[3] Readme / Help / Man`

The panel has three sources; the current one is shown in its title. On startup it
opens on the **README**: the repository README is fetched from the GitHub API and
rendered in the terminal (headings, lists, code blocks, tables). The mode is global,
not per tool — pick `--help` once and moving through the list keeps showing `--help`.

| Key | Action |
|-----|--------|
| `r` | README mode — the rendered repository README (the default); works only while `[3]` is focused, in `[1]` `r` is rename and in `[2]` refresh |
| `h` / `m` | `--help` / `man` mode (these two also work from `[2]`) |
| `j / k` | navigate by entries — flags and subcommands; the current entry is highlighted, the rest is dimmed (when there are no entries — in README mode, for example — `j / k` scroll 3 lines like the arrows) |
| `↑ / ↓` | scroll the text (3 lines) |
| `ctrl+d / ctrl+u`, `ctrl+f / ctrl+b`, `PgUp / PgDn`, `space`, `g / G` | half-page / full-page scroll, top / bottom |
| `/` | search the text (`n` / `N` — next / previous match); not available in README mode |
| `?` | hotkeys overlay |
| `esc` | first turns off entry navigation, then moves focus away |

The README is loaded lazily — one request per tool, cached for 24 hours — and only
for the tool whose README you actually look at: while you stay in `--help` or `man`
mode nothing is fetched at all. In README mode, though, moving to a tool for the first
time does spend that one request, so walking a long list on a cold cache costs one
request per tool visited. A tool without a `github` field, a repository without a
README, an exhausted quota or a failed fetch show a message with the way out
(`No repo for <name>`, `No README in <owner/repo>`, `rate limited — press [L]`,
`No README for <name>`); `r` in the brief panel re-fetches, bypassing the cache, and
adding a token in the `L` overlay retries the ones that hit the limit.

While a tool is being updated, this panel (`[3] Update`) shows the live command log;
the log stays available after completion — until the next update.

## Updating tools

![in-TUI update — detect the manager, confirm the command, stream the log into panel [3]](demo/update.gif)

When the installed version lags behind the latest release (the `↑` marker), press `u`
in the brief panel. `keeptui` detects the package manager the binary was installed with:

- `brew` — a `/Cellar/<formula>/…` path → `brew upgrade <formula>`;
- `go` — buildinfo (`go version -m`) with a `path` field → `go install <module>@latest`;
- `cargo` — a binary in `~/.cargo/bin` → `cargo install <crate>`;
- `pipx` — a venv in `~/.local/pipx/venvs/<pkg>/` → `pipx upgrade <pkg>`;
- `npm` — a global `node_modules/<pkg>` → `npm install -g <pkg>`.

The command is shown in the status bar for confirmation (`enter` runs it, any other
key cancels); its output streams into panel `[3] Update` in real time and the TUI
stays responsive. After a successful update the version is re-detected, the `↑`
marker disappears, and the tool leaves the update group. One update runs at a time;
a command gets 10 minutes (a sudo password prompt inside it fails fast instead of
hanging).

If the manager cannot be detected (manual install), `keeptui` suggests setting the
`update_cmd` field or updating manually (`o` opens the releases page). `update_cmd`
in `meta.yaml` always takes precedence over auto-detection and runs via `sh -c`
(pipes and `&&` are fine):

```yaml
- name: mytool
  github: github.com/owner/mytool
  update_cmd: mytool self-update
```

## GitHub API and token

`keeptui` fetches releases and repository cards through the GitHub REST API. Without a
token the limit is **60 requests per hour** per IP, with a token — **5000**. Each
tool with a `github` field costs 3 requests on startup, plus one more when you open
its README in panel `[3]`; so a cold start with a large list and no token can hit the
limit — cards stay empty until the window resets.

Quota usage is visible in the right corner of the status bar (`▮▮▮░░░ 12/60`). The
`L` key works from any panel (as long as no other input mode is active) and opens the
API status overlay: token source, quota usage with an icon (`⚠` — low, `✕` —
exhausted) and the reset time. Right in the overlay:

- `e` — enter a token (echo hidden); the token is validated with a `/rate_limit` request and saved only on success;
- `d` — remove the saved token (available only for the file-based token);
- `r` — refresh the numbers; `esc` / `q` — close.

The token source follows environment precedence: the `GITHUB_TOKEN` variable always
wins over the file. A token entered in the TUI is stored in `~/.config/keeptui/token`
with `0600` permissions; an environment token is never written to disk. When the
quota is exhausted, already-loaded cards are not erased, and a card with no data
shows the `rate limited — press [L]` hint.

## Data storage

The tool list lives in `~/.config/keeptui/meta.yaml` — one entry per tool (`name`,
`status`, `added`, optionally `tags`, `note`, `github`, `update_cmd`). The file is
fully managed from the TUI; editing it by hand is not required but safe — writes are
atomic.

| What | Where |
|------|-------|
| Tracker metadata | `~/.config/keeptui/meta.yaml` |
| Version and README cache (24h TTL) | `~/.config/keeptui/cache.json` |
| GitHub token (`0600`) | `~/.config/keeptui/token` |
| Session error log | `~/.config/keeptui/logs/keeptui-<timestamp>.log` |

The log is created lazily — only on the first error. A session with no errors leaves
no file at all, so the presence of a file is itself the signal. The 20 most recent
logs are kept.

## Architecture

How the code is organized — the package graph, data flow, TUI state machine,
subprocess sandbox — is described in [ARCHITECTURE.md](ARCHITECTURE.md).

## Stack

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) — text input, viewport, spinner
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styling
- [Glamour](https://github.com/charmbracelet/glamour) — markdown rendering for the README panel
- [golang.org/x/mod/semver](https://pkg.go.dev/golang.org/x/mod/semver) — version comparison
- [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) — reading/writing `meta.yaml`

## Contributing

Bug reports and pull requests are welcome. Before submitting, run
`go test -race ./...` and `go vet ./...` — CI checks the same.

## License

MIT
