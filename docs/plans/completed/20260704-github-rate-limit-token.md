# GitHub API Rate Limits & Token Setup

## Overview

Make `keys` resilient to GitHub REST API rate limits and let users supply their own token without leaving the TUI.

**Problem:** unauthenticated GitHub REST API allows 60 requests/hour per IP; with a token it is 5000/hour. Each tool with a `GitHub` field costs 3 requests (`fetchRelease` + `fetchRepoInfo` + `fetchLanguages` in `GetRepoData`, `internal/version/github.go`). A cold start with ~20 tools and no token exhausts the quota, leaving cards empty with a bare `HTTP 403`. The `X-RateLimit-*` headers are never read.

**Solution:**
- Read rate-limit numbers (hybrid: response headers update in the background, `GET /rate_limit` gives on-demand truth without spending quota).
- Surface a signal in the status bar and a detailed API-status overlay.
- Let users enter a `GITHUB_TOKEN` in the TUI, validated before saving, stored in a `0600` file.
- Degrade gracefully on exhaustion (typed `ErrRateLimited`, keep known data).

**Benefits:** users understand why cards fail to load, can raise the limit 60→5000 in seconds, and never see a raw HTTP error.

## Context (from discovery)

- **Project:** Go terminal TUI (Bubble Tea) tracking CLI tools. Pure TUI, no CLI flags.
- **Files/areas involved:**
  - `internal/version/github.go` — all GitHub API calls; `os.Getenv("GITHUB_TOKEN")` duplicated in 3 fetch funcs (lines ~247, ~284, ~315); `GetRepoData` is the single network pass; `cacheTTL = 24h`.
  - `internal/model/model.go` — Bubble Tea model; `remoteMsg` struct (line 46); modal-mode pattern (`tracking`/`confirmingUntrack`/`editingNote`); `autoFetchCmdsForSelected()` (line 1484); key handling; `renderStatusBar()`.
  - `internal/ui` — Lip Gloss styles + `PlaceOverlay` (used by changelog popup).
  - `internal/loader/meta.go` — `os.UserConfigDir()` + `keys/` for config paths (line 44).
- **Patterns found:**
  - Modal modes are bool flags with an early branch in `Update()` and a matching branch in `renderStatusBar()`.
  - Overlays render via `ui.PlaceOverlay` (changelog popup precedent).
  - Async fetch: `Init()` fires `fetchInstalledCmd` + `fetchRemoteCmd` per tool; results arrive as messages; `autoFetchCmdsForSelected()` re-fetches for the selected tool.
  - Tests use `testAPIBase` to point HTTP calls at an `httptest` server (`internal/version/github_test.go`).
- **Key availability:** `g`/`G` are taken (vim top/bottom nav, model.go:456). **Overlay opens with `L`** (limits); `L` is free.
- **Dependencies:** `bubbletea`, `lipgloss`, `bubbles/textinput` (already used for note/tag editing).

## Development Approach

- **Testing approach:** Regular (code first, then tests) — matches existing package style with `testAPIBase`.
- Complete each task fully before the next; small focused changes.
- **Every task includes new/updated tests** as separate checklist items (success + error cases).
- **All tests must pass before starting the next task.**
- Update this plan when scope changes.
- Maintain backward compatibility: env-only setup keeps working unchanged.

## Testing Strategy

- **Unit tests:** required per task, in `internal/version/*_test.go` and `internal/model/*_test.go`, using `testAPIBase` + `httptest` for network, `t.TempDir()` + `t.Setenv` for token file paths.
- **e2e tests:** none — project has no browser/UI e2e harness. Model rendering is verified via string-output unit tests (existing `render_test.go` pattern).

## Progress Tracking

- Mark completed items `[x]` immediately.
- New tasks: ➕ prefix. Blockers: ⚠️ prefix.
- Keep plan in sync with actual work.

## Solution Overview

Two cooperating layers:

1. **`version` package (data + network):** owns token resolution, a shared `RateLimit` snapshot (mutex-guarded), a `doGH(req)` helper that centralizes auth header + rate-limit accounting, `FetchRate()` against `/rate_limit`, and a typed `ErrRateLimited`. Token persisted to `~/.config/keys/token` (`0600`) in a new `token.go`.
2. **`model` package (UI):** carries the `RateLimit` snapshot into the model via messages (`remoteMsg` gains a `rate` field; new `rateMsg` for `/rate_limit`), renders a status-bar signal and an `L`-triggered API-status overlay with token entry/removal/refresh.

**Key design decisions:**
- **Hybrid rate source:** headers are free but only refresh when a request happens; `/rate_limit` is on-demand truth and does not spend quota → used on overlay open + refresh + token validation.
- **msg-based wiring:** keeps the Bubble Tea model as the single source of UI truth (`m.rate`), rather than reading package state during render.
- **`doGH` abstraction (user-approved):** removes 3 copies of the auth header and gives one place to account for rate limits.
- **Separate token file (not meta.yaml):** avoids mixing a secret with shareable tracker data; `0600` perms.
- **Env precedence:** `GITHUB_TOKEN` env always wins over the config file; `[remove token]` only offered for the config source.

## Technical Details

**Data structures (`internal/version`):**
```go
type RateLimit struct {
    Limit     int       // 60 unauth, 5000 with token
    Remaining int
    Reset     time.Time
    Known     bool      // any successful observation yet
}

var ErrRateLimited = errors.New("github api rate limit exceeded")

// token state
var (
    tokenMu  sync.RWMutex
    tokenMem string // from config file or TUI entry
)

// rate state
var (
    rlMu sync.RWMutex
    rl   RateLimit
)
```

**Token resolution:** `resolveToken()` returns env token if set, else `tokenMem`. On first use it lazily loads the config file **once** via `sync.Once` (`loadTokenOnce.Do(loadTokenFromFile)`), and all `tokenMem` reads/writes go through `tokenMu` — required because `resolveToken()` runs concurrently from every startup `doGH` goroutine and must be `-race` clean. `SetToken(t)` updates `tokenMem` (under lock) and writes the file `0600`. `TokenSource()` returns `"env"|"config"|"none"`. `ClearToken()` removes file + `tokenMem`. Token file path via `os.UserConfigDir()` + `keys/token` (self-contained in `token.go`).

**doGH:** `func doGH(req *http.Request) (*http.Response, error)` — sets `Accept` + `Authorization: Bearer <resolveToken()>` (only if non-empty), does the request with the 5s-timeout client, calls `updateRateFromHeaders(resp.Header)`, and returns. Callers keep their own status-code handling and pass the response to `classifyStatus`.

**classifyStatus:** `func classifyStatus(resp *http.Response) error` — 403/429 → returns `ErrRateLimited` **only when `resp.Header` (this response's own headers) reports `X-RateLimit-Remaining == 0`**; a 403 with remaining>0 (genuine no-access) returns a generic `HTTP %d`. **Reads Remaining from the response headers, never from global `rl`** — under concurrent startup another goroutine can overwrite `rl` between this request's `updateRateFromHeaders` and its classification, so the per-response header is the only correct source.

**updateRateFromHeaders:** parses `X-RateLimit-Limit`/`-Remaining`/`-Reset` (Reset is unix seconds); ignores missing/malformed headers (leaves `rl` untouched, sets `Known=true` only when parsed). Guarded by `rlMu`.

**FetchRate / validation:** `FetchRate() (RateLimit, error)` hits `GET /rate_limit`, decodes `resources.core`, updates `rl`, returns snapshot; does not spend core quota. For **token validation before persistence**, add `FetchRateWithToken(token string) (RateLimit, error)` (or `validateToken(token)`), which issues the `/rate_limit` request with an explicit `Authorization: Bearer <token>` **without touching global `tokenMem`/the file**. 401 → validation error. `SetToken` is called **only after** a 200 from this candidate check, so an invalid token is never written to disk and no concurrent fetch ever reads it.

**Model wiring:** `remoteMsg` gains `rate version.RateLimit`; `fetchRemoteCmd` reads `version.Rate()` right after the network call and attaches it. New `rateMsg{rate version.RateLimit; err error}` from a `fetchRateCmd`. Model field `m.rate version.RateLimit`. The `remoteMsg`/`rateMsg` handlers **merge**: a snapshot with `Known==false` (e.g. a cache-hit `remoteMsg` where no request was made) must **not** overwrite a previously-known `m.rate`. New modal flag `showingAPIStatus` and, during token entry, reuse the existing `textinput` pattern (masked echo).

**Warm-cache signal caveat:** `GetRepoData` returns from the 24h cache without any network request (github.go:109); in that path no headers are observed and `Rate()` stays `Known==false`. So on a warm start the passive status-bar signal reflects only the **last real request**. To populate it, `Init()` fires one `fetchRateCmd` at startup (cheap, does not spend quota), and the non-clobber merge preserves it across cache-hit `remoteMsg`s.

**ErrRateLimited consumer:** `fetchRemoteCmd` inspects the error from `GetRepoData`/`GetLatest`; when `errors.Is(err, version.ErrRateLimited)` **and** there is no cached card, it sets a distinct `remoteMsg.repoStatus` hint (e.g. `"rate-limited"`) so the card renders "rate limited — press [L]" instead of a bare failure. This gives the typed error a real runtime consumer (not just a classified/tested value).

**Processing flow (token entry):** `L` (guarded — ignored while any input/modal mode is active) → set `showingAPIStatus`, fire `fetchRateCmd` → overlay shows fresh numbers → `[e]` opens masked textinput → submit runs `FetchRateWithToken(candidate)` → 401 sets an inline "token invalid" message and discards (nothing written); 200 calls `SetToken` then triggers `autoFetchCmdsForSelected()` to backfill cards.

## What Goes Where

- **Implementation Steps (checkboxes):** all code, tests, and doc updates in this repo.
- **Post-Completion (no checkboxes):** manual token round-trip verification, rate-limit-exhaustion behavior against the live API.

## Implementation Steps

### Task 1: Token resolution and persistence

**Files:**
- Create: `internal/version/token.go`
- Create: `internal/version/token_test.go`

> Note: the 3 inline `os.Getenv("GITHUB_TOKEN")` header sites in `github.go` are **not** touched here — they are removed in Task 2 when `doGH` becomes the single auth point (avoids editing the same lines twice).

- [x] add `tokenMu` (`sync.RWMutex`) + `tokenMem` state and `resolveToken()` (env precedence, then `tokenMem`)
- [x] add `tokenFilePath()` via `os.UserConfigDir()` + `keys/token`
- [x] add `SetToken(t)` (update `tokenMem` under lock, write file `0600`, `MkdirAll` for dir), `ClearToken()`, `TokenSource() string`
- [x] add `loadTokenFromFile()` invoked via a package `sync.Once` on the first `resolveToken()` so env-empty startup picks up the file exactly once and `-race` clean (all `tokenMem` access through `tokenMu`)
- [x] write tests: `resolveToken` env-over-config precedence + empty cases (`t.Setenv`, `t.TempDir` for config dir)
- [x] write tests: `SetToken` writes file with `FileMode 0600` (assert `Stat().Mode().Perm()`), `ClearToken` removes it, `TokenSource` returns env/config/none
- [x] run tests (add `-race`) — must pass before task 2

### Task 2: RateLimit state, doGH helper, and header accounting

**Files:**
- Modify: `internal/version/github.go`
- Create/Modify: `internal/version/github_test.go`

- [x] add `RateLimit` struct, `rlMu`/`rl` state, `Rate() RateLimit` snapshot accessor
- [x] add `updateRateFromHeaders(http.Header)` parsing `X-RateLimit-*` (unix Reset), tolerant of missing/malformed values
- [x] add `doGH(req) (*http.Response, error)` setting `Accept` + optional `Authorization` (via `resolveToken()`), doing the request with the 5s client, calling `updateRateFromHeaders`
- [x] refactor `fetchRelease`/`fetchRepoInfo`/`fetchLanguages` to build the request then call `doGH` (drop duplicated header/client code)
- [x] write tests: `updateRateFromHeaders` valid headers, missing headers (state untouched), malformed values
- [x] write tests: `doGH` sends `Authorization` when token set and omits it when empty (assert on `httptest` server); confirm the 3 fetchers still parse correctly via `testAPIBase`
- [x] run tests — must pass before task 3

### Task 3: FetchRate and ErrRateLimited classifier

**Files:**
- Modify: `internal/version/github.go`
- Modify: `internal/version/github_test.go`

- [x] add `var ErrRateLimited = errors.New(...)` and `classifyStatus(resp) error` — 403/429 → `ErrRateLimited` **only when `resp.Header`'s `X-RateLimit-Remaining==0`** (read from this response, NOT global `rl`); other non-2xx → generic `HTTP %d`
- [x] use `classifyStatus` in the 3 fetchers, replacing the ad-hoc rate-limit string in `fetchRelease` and the bare `HTTP 403` in `fetchRepoInfo`/`fetchLanguages` (preserve `404 → no releases`)
- [x] add `FetchRate() (RateLimit, error)` hitting `/rate_limit`, decoding `resources.core`, updating `rl`
- [x] add `FetchRateWithToken(token string) (RateLimit, error)` — same request but with an explicit `Authorization` header, **without** touching global `tokenMem`/file; 401 → distinct validation error (used by Task 7 to validate before persisting)
- [x] confirm `GetRepoData` still keeps known tag/card on total failure (no behavior change)
- [x] write tests: `FetchRate` parses `resources.core` (mock `/rate_limit`); `FetchRateWithToken` sends the given token and returns validation error on 401, and does NOT mutate `tokenMem`
- [x] write tests: `classifyStatus` — 403 with header `Remaining==0` → `errors.Is(err, ErrRateLimited)`; 403 with header `Remaining>0` → generic error (assert classification uses the response header, not global state)
- [x] run tests — must pass before task 4

### Task 4: Carry RateLimit into the model via messages

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/*_test.go` (message/handler tests)

- [x] add `rate version.RateLimit` field to `remoteMsg`; set it from `version.Rate()` in `fetchRemoteCmd` after the network call
- [x] set `remoteMsg.repoStatus` hint (e.g. `"rate-limited"`) when `errors.Is(err, version.ErrRateLimited)` and no cached card — gives `ErrRateLimited` a real consumer; card renders "rate limited — press [L]"
- [x] add `rateMsg{rate version.RateLimit; err error}` and `fetchRateCmd()` calling `version.FetchRate()`
- [x] fire `fetchRateCmd()` from `Init()` so the status-bar signal is populated on warm-cache starts (cache-hit `remoteMsg`s carry `Known==false`; this seeds `m.rate` once)
- [x] add `m.rate version.RateLimit`; in the `remoteMsg`/`rateMsg` handlers **merge** — a `Known==false` snapshot must NOT overwrite a previously-known `m.rate`
- [x] write tests: `remoteMsg` handler stores a `Known` rate and does not clobber a known `m.rate` with `Known==false`; `rateMsg` handler stores `rate` and surfaces error; `ErrRateLimited` sets the status hint
- [x] run tests — must pass before task 5

### Task 5: Status-bar rate-limit signal

**Files:**
- Modify: `internal/model/model.go` (`renderStatusBar`)
- Modify: `internal/ui` (add `⚠`/`✕` warning/danger styles if none fit)
- Modify: `internal/model/render_test.go`

- [x] add `const rateLowThreshold = 10`
- [x] in `renderStatusBar()` read `m.rate`: normal (quiet or `GH R/L`), warning `Remaining<=threshold` (yellow `⚠ GH R/L · [L] details`), exhausted `Remaining==0` (red `✕ GH limit exhausted · [L]`); render nothing when `!Known` (note: on warm starts this is seeded by the `Init()` `fetchRateCmd` from Task 4)
- [x] add Lip Gloss warn/danger styles in `internal/ui` if not already present
- [x] write tests: status bar renders no signal when `!Known`, warning icon at threshold, danger icon at zero
- [x] run tests — must pass before task 6

### Task 6: API-status overlay (read-only view + refresh)

**Files:**
- Modify: `internal/model/model.go` (`showingAPIStatus` flag, `Update`, `View`, `renderStatusBar`)
- Modify: `internal/model/render_test.go`

- [x] add `showingAPIStatus` flag; `L` key opens the overlay and fires `fetchRateCmd` — **guarded**: handle `L` only when no input/modal mode is active (`tracking`/`confirmingUntrack`/`renaming`/`editingNote`/`editingTags`/search and, inside the overlay, the token-input sub-mode)
- [x] render overlay via `ui.PlaceOverlay` — English labels: `Token: <source> (masked)`, `Limit: <icon> R / L`, `Reset: in X min (HH:MM)`, action hints `[e] set token  [d] remove token  [r] refresh  [esc] close`
- [x] icon next to `Limit` uses `rateLowThreshold` (none / `⚠` / `✕`) — single source of truth shared with status bar
- [x] `[r]` refresh → `fetchRateCmd` (updates `m.rate`, overlay stays open); `[esc]` closes; add matching `renderStatusBar()` branch for the overlay mode
- [x] show `[d]` only when `TokenSource()=="config"`
- [x] add token-masking helper (`ghp_••••••••3f2a`, first 4 + last 4)
- [x] write tests: overlay renders masked token + limit line + correct icon; `[d]` hidden for env source; masking helper output
- [x] run tests — must pass before task 7

### Task 7: Token entry, validation, and removal in the overlay

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/model/render_test.go`

- [x] `[e]` enters token-input mode using `textinput` with masked echo (reuse note/tag editing pattern)
- [x] on submit: validate via `version.FetchRateWithToken(candidate)` — **without persisting**; 401 → inline "token invalid" message, discard (nothing written), stay in overlay
- [x] on valid (200): call `version.SetToken` (this is the first and only write to disk), then trigger `autoFetchCmdsForSelected()` to backfill cards; refresh overlay numbers
- [x] `[d]` (config source only): `version.ClearToken()`, refresh overlay
- [x] handle the token-input mode in `renderStatusBar()` (mirror editing-input pattern)
- [x] write tests: valid token → stored + backfill cmd fired; invalid token (401) → NOT stored (assert token file absent/unchanged) + error shown; `[d]` clears token
- [x] run tests — must pass before task 8

### Task 8: Verify acceptance criteria

- [x] verify all Overview requirements: hybrid rate read, status-bar signal, overlay, in-TUI token entry with validation, `0600` storage, graceful degradation
- [x] verify edge cases: env token present (config `[d]` hidden), no token + exhausted limit (danger signal, no modal error), malformed `/rate_limit` response
- [x] run `go build .`
- [x] run `go vet ./...`
- [x] run full test suite: `go test ./...`

### Task 9: Update documentation

- [x] update `CLAUDE.md` GitHub API section (limits 60/5000, `doGH`, hybrid rate read, `FetchRate`, `ErrRateLimited`, `L` overlay, token entry)
- [x] update `CLAUDE.md` File storage table (add `~/.config/keys/token`, `0600`) and TUI state-machine section (`L` opens API-status overlay)
- [x] update `README.md` if it documents `GITHUB_TOKEN` setup (README did not document it; added a GitHub API / token section, `L` key, and token storage row)
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

*Items requiring manual intervention or external systems — informational only*

**Manual verification:**
- Round-trip a real token in the TUI: enter → confirm `Limit` jumps to `.../5000`, confirm `~/.config/keys/token` exists with `0600`, restart app and confirm token loads from file.
- Enter a deliberately bad token → confirm "token invalid" and nothing saved.
- Exhaust the unauthenticated limit (or simulate) → confirm status-bar danger signal and that existing cards are not wiped.

**Notes:**
- Secondary/abuse rate limits, token scopes display, GraphQL, auto-retry after Reset, and token-file encryption are intentionally out of scope (YAGNI).
