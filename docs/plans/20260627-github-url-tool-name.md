# Fix: derive short tool name when tracking a GitHub URL

## Overview
- When a user runs `keys track <github-url>`, the full URL lands in `ToolMeta.Name`
  verbatim (`internal/cmd/track.go:16,62`). That URL is then shown in the left tool list
  and, worse, used as an executable command name in three places — breaking man/help and
  version detection.
- This plan makes `keys track` recognise a GitHub reference passed as the first argument,
  derive a short tool name from it (the `repo` segment of `owner/repo`, without a trailing
  `.git`), store the short name in `Name`, and put the normalized repo into the `github`
  field.
- Benefit: the tool list shows a clean name (e.g. `neovim`), and `man`, `--help`,
  `--version` resolve a real command instead of a URL.

## Context (from discovery)
- Files/components involved:
  - `internal/cmd/track.go` — the only place tools are added (`RunTrack`).
  - `internal/loader/meta.go` — `ToolMeta` struct (`Name`, `GitHub`), `FindMeta`, `UpsertMeta`.
  - `internal/version/github.go:381` — existing `extractRepo` (URL → `owner/repo`), used only
    for API calls; lives in `version` which `loader` cannot import (cycle: `version` → `loader`).
  - `internal/model/model.go:849` renders `mt.Name`; `model.go:1496,1508` run `man name` /
    `name --help`; `internal/version/detect.go:23-24` run `name --version`.
- Related patterns: table-driven tests in `internal/version/github_test.go` (style to mirror).
- Decisions already agreed with user:
  - GitHub URL → auto-derive name (do NOT reject with an error).
  - No migration of existing "broken" `meta.yaml` entries — fix the source only; the user
    re-tracks old entries locally.

## Development Approach
- **testing approach**: Regular (code first, then tests).
- complete each task fully before moving to the next.
- make small, focused changes.
- **every task includes new/updated tests** for code changed in that task (success + error/edge).
- **all tests must pass before starting the next task**.
- update this plan file if scope changes during implementation.
- maintain backward compatibility (plain names like `git` keep working unchanged).

## Testing Strategy
- **unit tests**: required per task. Table-driven tests for `ParseToolRef` and `NormalizeRepo`
  in `internal/loader/github_test.go`.
- **e2e tests**: project has no UI e2e harness (TUI only) — none required. Manual TUI check
  is listed under Post-Completion.

## Progress Tracking
- mark completed items with `[x]` immediately when done.
- add newly discovered tasks with ➕ prefix; blockers with ⚠️ prefix.
- keep this plan in sync with actual work.

## Solution Overview
- Add a small parsing helper to the `loader` package (which `track.go` already imports),
  so there is no import cycle.
- `loader.ParseToolRef(arg)` classifies the track argument: a GitHub reference yields a short
  name + normalized `github.com/owner/repo`; anything else is returned as a plain name.
- `loader.NormalizeRepo(s)` holds the shared URL→`owner/repo` logic; `version.extractRepo`
  delegates to it to remove duplication (`version` already imports `loader`).
- `track.go` runs `ParseToolRef` before any lookup/save so `Name` is always a short name and
  the URL only ever lives in the `github` field. No changes needed in the man/help/version
  call sites — they become correct automatically.

## Technical Details

### `loader.ParseToolRef(arg string) (name, github string, isGitHub bool)`
- `isGitHub` when `strings.Contains(arg, "github.com")` OR `arg` has prefix `http://`/`https://`.
- Parse: strip scheme and `github.com/` prefix, trim leading/trailing `/`, split on `/`.
  Need `parts[0]` (owner) and `parts[1]` (repo); ignore extra tail like `/tree/main`.
- `name = strings.TrimSuffix(parts[1], ".git")`.
- `github = "github.com/" + owner + "/" + name`.
- If it looks like a GitHub ref but cannot be parsed into two segments, return
  `(arg, "", false)` (treat as plain name — never lose the input).

### `loader.NormalizeRepo(s string) string`
- Same trimming as the current `version.extractRepo`: strip `https://`/`http://`/`github.com/`,
  split, return `owner/repo` or `""` when invalid. `ParseToolRef` reuses this internally.

### `track.go` wiring
- Replace `name := args[0]` with `name, ghFromArg, isGitHub := loader.ParseToolRef(args[0])`.
- Use `name` for `FindMeta`, `entry.Name`, and the final `fmt.Printf`.
- GitHub field precedence: explicit `--github` wins; otherwise set `ghFromArg` when `isGitHub`,
  in both the existing-entry and new-entry branches.

## What Goes Where
- **Implementation Steps**: helper + tests, track.go wiring, dedup of `extractRepo`, verification.
- **Post-Completion**: manual TUI verification with a throwaway HOME.

## Implementation Steps

### Task 1: Add GitHub-ref parsing helpers to the loader package

**Files:**
- Create: `internal/loader/github.go`
- Create: `internal/loader/github_test.go`

- [x] add `NormalizeRepo(s string) string` to `internal/loader/github.go` (URL/path → `owner/repo`)
- [x] add `ParseToolRef(arg string) (name, github string, isGitHub bool)` reusing `NormalizeRepo`
- [x] write table-driven tests for `ParseToolRef`: `https://github.com/neovim/neovim` → (`neovim`,
      `github.com/neovim/neovim`, true); `github.com/junegunn/fzf` → (`fzf`, ..., true);
      `https://github.com/sharkdp/bat.git` → (`bat`, `github.com/sharkdp/bat`, true);
      `https://github.com/owner/repo/tree/main` → (`repo`, `github.com/owner/repo`, true)
- [x] write tests for plain/edge inputs: `git` → (`git`, "", false); `tmux` → (`tmux`, "", false);
      malformed github-ish input → returns the original arg, "", false
- [x] add a short test for `NormalizeRepo` (valid + invalid cases)
- [x] run `go test ./internal/loader/...` — must pass before next task

### Task 2: Use ParseToolRef in `keys track`

**Files:**
- Modify: `internal/cmd/track.go`

- [ ] replace `name := args[0]` with `name, ghFromArg, isGitHub := loader.ParseToolRef(args[0])`
- [ ] ensure `FindMeta`, `entry.Name`, and the success `fmt.Printf` all use the derived `name`
- [ ] set GitHub field with precedence: `--github` flag wins, else `ghFromArg` when `isGitHub`
      (apply in both existing-entry and new-entry branches)
- [ ] verify `usage`/`-` guard still rejects a bare flag as first arg (unchanged behaviour)
- [ ] run `go build ./...` and `go vet ./...` — must pass before next task

### Task 3: Remove duplication — delegate `extractRepo` to `loader.NormalizeRepo`

**Files:**
- Modify: `internal/version/github.go`

- [ ] change `extractRepo` (line ~381) to call `loader.NormalizeRepo` (keep the name/signature
      so the five existing call sites are untouched)
- [ ] confirm `internal/version/github_test.go` still covers the normalization behaviour;
      add/keep a case if coverage moved
- [ ] run `go test ./internal/version/...` — must pass before next task

### Task 4: Verify acceptance criteria
- [ ] tracking a GitHub URL stores a short `Name` and a `github.com/owner/repo` github field
- [ ] tracking a plain name (`git`) is unchanged
- [ ] run full test suite: `go test ./...`
- [ ] run `go vet ./...` and `go build ./...`

### Task 5: Update documentation
- [ ] update `CLAUDE.md` only if a new convention is worth recording (note: `loader` now owns
      GitHub-ref parsing)
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention — informational only*

**Manual verification:**
- In a throwaway config dir (e.g. `XDG_CONFIG_HOME`/`HOME` pointed at a temp dir so the real
  `~/.config/keys/meta.yaml` is untouched):
  - `keys track https://github.com/neovim/neovim` → `keys list` shows `neovim`, not the URL.
  - inspect `meta.yaml`: `name: neovim`, `github: github.com/neovim/neovim`.
  - launch the TUI (`go run .`): left list shows the short name; for a tool whose binary is
    installed, man/help opens without a "command not found" error.
