# ghnotify — spec

## Summary

A macOS menubar app (background process) that monitors GitHub pull requests across
github.com and arbitrary GitHub Enterprise Server instances. Written in Go.
Ships via goreleaser. First target: macOS.

---

## Tech stack

| Concern | Library |
|---|---|
| Menubar | `github.com/getlantern/systray` |
| CLI bootstrap | `github.com/urfave/cli/v3` |
| GitHub API | `github.com/google/go-github/v62` + `golang.org/x/oauth2` |
| Keychain | `github.com/zalando/go-keyring` (wraps macOS Keychain, libsecret on Linux) |
| Config file | `github.com/BurntSushi/toml` |
| HTTP server (OAuth callback) | stdlib `net/http` |
| OS notifications | `github.com/gen2brain/beeep` |
| Releases | goreleaser |
| Update check | `github.com/inconshreveable/go-update` or direct GitHub Releases poll |

---

## Repository layout

```
ghnotify/
├── main.go                    # entry point: urfave/cli setup, systray.Run()
├── .goreleaser.yml
├── internal/
│   ├── config/
│   │   ├── config.go          # ServerConfig, AppConfig, load/save TOML
│   │   └── paths.go           # platform-specific config/data dirs
│   ├── auth/
│   │   ├── keychain.go        # get/set/delete via go-keyring
│   │   ├── oauth.go           # OAuth2 PKCE flow, local HTTP callback server
│   │   └── pat.go             # PAT storage and validation
│   ├── github/
│   │   ├── client.go          # per-server *github.Client factory
│   │   ├── prs.go             # fetch my PRs + review requests
│   │   └── review.go          # submit approve
│   ├── poller/
│   │   ├── poller.go          # per-server poll loop goroutine
│   │   └── state.go           # in-memory + disk state (snooze, seen, counts)
│   ├── tray/
│   │   ├── tray.go            # systray menu builder/updater
│   │   └── menu.go            # menu item factories (PR item, server item)
│   ├── notify/
│   │   └── notify.go          # OS notification dispatch + filtering by prefs
│   └── updater/
│       └── updater.go         # poll GitHub Releases, compare semver
└── tasks/
    └── spec.md                # this file
```

---

## Configuration

Stored at:
- macOS: `~/Library/Application Support/ghnotify/config.toml`
- Linux: `~/.config/ghnotify/config.toml`

```toml
poll_interval = "60s"
max_prs_per_section = 20   # per "My PRs" and "Review Requests" sections

[notifications]
new_review_requests = true
pr_approved         = true
pr_merged           = true
ci_status_change    = false
new_comments        = false

[[servers]]
host        = "github.com"
auth_type   = "oauth"           # "oauth" | "pat"
client_id   = "Ov23li..."       # OAuth App client ID (not secret — that's in Keychain)

[[servers]]
host        = "github.mycompany.com"
auth_type   = "pat"             # PAT for unregistered GHE instances

# GHE with user-supplied OAuth App:
[[servers]]
host        = "github.other.com"
auth_type   = "oauth"
client_id   = "abc..."          # user-registered OAuth App on that GHE instance
```

---

## Keychain storage

Service name: `"ghnotify"`, account format: `"<host>:<key>"`

| Account key | Contents |
|---|---|
| `github.com:access_token` | OAuth access token |
| `github.com:refresh_token` | OAuth refresh token (if issued) |
| `github.com:oauth_client_secret` | OAuth App client secret |
| `github.mycompany.com:pat` | Personal access token |

---

## Authentication flows

### OAuth (primary, github.com + pre-registered GHE)

1. User clicks "Login" for a server.
2. App generates PKCE `code_verifier` + `code_challenge`.
3. Opens browser: `https://<host>/login/oauth/authorize?client_id=...&scope=repo,read:org&...`
4. Spins up `net/http` listener on a random available `localhost:PORT`.
5. GitHub redirects to `http://localhost:PORT/callback?code=...`.
6. App POSTs to `https://<host>/login/oauth/access_token` with code + verifier.
7. Stores `access_token` (+ `refresh_token` if present) in Keychain.
8. Local server shuts down.

Token refresh: if the access token is expired and a refresh token exists, silently
exchange it before each API call. Update Keychain on success.

### PAT (fallback for unregistered GHE)

1. User clicks "Set PAT…" for a server.
2. A dialog prompts for the token (systray doesn't support native dialogs — use
   a small terminal prompt via `osascript` on macOS, or read from stdin on Linux).
3. Token validated immediately via `GET /user`.
4. Stored in Keychain under `<host>:pat`.

---

## PR data model

```go
type PR struct {
    Server      string     // host, e.g. "github.com"
    Owner       string     // org or user
    Repo        string     // repo name
    Number      int
    Title       string
    URL         string
    Author      string
    IsDraft     bool
    ReviewState ReviewState  // Pending | Approved | ChangesRequested | Dismissed
    CIStatus    CIStatus     // Unknown | Pending | Passing | Failing
    Mergeable   Mergeability // Mergeable | Conflicted | Unknown
    CommentCount int
    UpdatedAt   time.Time
}
```

---

## Polling loop

One goroutine per server, managed by `poller.Manager`.

Each tick:
1. Fetch `GET /user/pulls?state=open&filter=created` → my PRs.
2. Fetch via search API: `is:open is:pr review-requested:@me` → review requests.
3. For each PR, fetch:
   - Reviews (to compute aggregate `ReviewState`)
   - Check runs / statuses (aggregate `CIStatus`)
   - PR detail (mergeable, comment count, updated_at)
4. Diff against previous state:
   - New PR → add to list, notify if configured.
   - PR removed (merged/closed) → remove from list.
   - Field changed → update list item; fire notification based on what changed + user prefs.
5. Apply snooze filter before rendering menu.
6. Push updated state to tray via a channel.

Interval: configurable via `poll_interval` in config (default 60s).
Jitter: ±10% random jitter per tick to avoid thundering herd across multiple servers.

---

## Snooze / state persistence

State file: `~/Library/Application Support/ghnotify/state.json` (macOS)

```json
{
  "snoozed": {
    "github.com/owner/repo#123": {
      "mode": "until_change",
      "snapshot_updated_at": "2026-06-05T10:00:00Z"
    },
    "github.com/owner/repo#456": {
      "mode": "until_time",
      "expires_at": "2026-06-05T18:00:00Z"
    }
  }
}
```

Snooze is lifted when:
- `until_change`: `PR.UpdatedAt > snapshot_updated_at`
- `until_time`: `now > expires_at`

---

## Menubar structure

```
[icon]  (badge = total count of: review requests + my PRs needing attention)

  ● My Pull Requests  (N)
    ├── github.com › owner/repo
    │   ├── [✓CI][Approved] #123 Add feature X
    │   │   ├── Open in Browser
    │   │   ├── Copy URL
    │   │   ├── Approve
    │   │   ├── Snooze until next change
    │   │   └── Snooze ▶
    │   │       ├── 1 hour
    │   │       ├── 8 hours
    │   │       └── 24 hours
    │   └── [✗CI][Draft] #124 WIP: refactor
    └── github.enterprise.com › org/repo
        └── ...

  ─────────────────────────────

  ● Review Requests  (N)
    ├── github.com › owner/repo
    │   └── [?CI][Changes Requested] #99 Fix bug Y
    │       ├── Open in Browser
    │       ├── Copy URL
    │       ├── Approve
    │       ├── Snooze until next change
    │       └── Snooze ▶  (1h / 8h / 24h)
    └── ...

  ─────────────────────────────

  Servers
    ├── github.com  [Connected as @mattv]
    │   ├── Refresh now
    │   └── Logout
    └── github.enterprise.com  [Not connected]
        ├── Login with OAuth…
        ├── Set PAT…
        └── Remove server

  Add server…
  Preferences…   (opens config.toml in $EDITOR / TextEdit)
  Check for updates
  About  (shows version)
  Quit
```

PR line format (fixed-width emoji columns):
`[CI][Review] #NNN  Title (truncated)`

CI glyphs: `✓` passing, `✗` failing, `◌` pending, `·` unknown  
Review glyphs: `✓` approved, `✗` changes requested, `◌` awaiting, `~` draft

---

## Notifications

Dispatched via `beeep` (wraps `osascript` on macOS → native notification center).

Configurable per-event type in `[notifications]` block. Events:

| Event | Default |
|---|---|
| New review request | on |
| My PR approved | on |
| My PR merged | on |
| CI status changed | off |
| New comment | off |

Notification body: `"[owner/repo] #NNN: <event description>"`  
Click action: open PR in browser (via notification URL if beeep supports it, else
fall back to menubar interaction).

---

## Update checker

On startup and every 24 hours:
1. `GET https://api.github.com/repos/boyvinall/ghnotify/releases/latest`
2. Compare `tag_name` semver vs build-time `version` constant (injected by goreleaser via `-ldflags`).
3. If newer: show "Update available: v1.x.x — Download" item in menubar.

---

## goreleaser (.goreleaser.yml outline)

```yaml
version: 2

builds:
  - id: ghnotify
    main: .
    binary: ghnotify
    env: [CGO_ENABLED=1]   # systray requires cgo on macOS
    goos: [darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}

universal_binaries:
  - id: ghnotify-universal
    ids: [ghnotify]
    replace: true

archives:
  - format: zip

# Code signing (requires local Developer ID cert in Keychain):
hooks:
  post:
    - cmd: codesign --deep --force --verify --verbose
             --sign "Developer ID Application: Matt Vosburgh (TEAMID)"
             --options runtime
             dist/ghnotify_darwin_all/ghnotify

# Notarization (requires APPLE_ID + APP_PASSWORD + TEAM_ID env vars):
    - cmd: |
        xcrun notarytool submit dist/ghnotify_darwin_all/ghnotify.zip \
          --apple-id "$APPLE_ID" --password "$APP_PASSWORD" --team-id "$TEAM_ID" --wait
      env: [APPLE_ID, APP_PASSWORD, TEAM_ID]

release:
  github:
    owner: boyvinall
    name: ghnotify
```

Code signing is conditional: if `Developer ID Application` cert is absent from
Keychain, the codesign hook exits 0 with a warning (wrapping script).

---

## Open questions / decisions deferred to implementation

1. **"Add server" UX**: systray has no native text input. Options:
   - `osascript -e 'display dialog "Enter GHE hostname:"'` on macOS.
   - A minimal web page served locally (localhost:PORT) that the app opens in a browser.
   - Write a hostname to a well-known file and watch it.
   Recommendation: `osascript` dialog on macOS for v1; abstract behind an `inputDialog()` interface for cross-platform later.

2. **"Set PAT" UX**: same constraint — same solution (osascript dialog).

3. **systray icon**: needs a template image (black/white with alpha) for macOS dark/light mode. Provide both a static icon and a "has updates" variant.

4. **Preferences**: open `config.toml` in `$EDITOR` via `os/exec`. Show a "restart required" notice in the menubar after edits are detected (via `fsnotify`).

5. **Multiple accounts on the same host**: out of scope for v1 — one account per host.

6. **GraphQL vs REST**: use REST for v1 (simpler); GraphQL would reduce round-trips for status aggregation but adds complexity.

---

## Implementation phases

### Phase 1 — skeleton ✓
- [x] `go mod init github.com/boyvinall/ghnotify`
- [x] `urfave/cli/v3` bootstrap, `systray.Run()` with static icon
- [x] Config load/save (TOML)
- [x] Keychain read/write wrapper

### Phase 2 — auth ✓
- [x] OAuth PKCE flow (localhost callback)
- [x] PAT storage + validation
- [x] Token refresh for OAuth
- [x] Server management menu items (Login / Set PAT / Logout / Remove)

### Phase 3 — polling ✓
- [x] GitHub client factory (per server, token injected)
- [x] Fetch my PRs + review requests
- [x] Aggregate review state, CI status, merge readiness, comment count
- [x] Poll loop with configurable interval + jitter
- [x] State diff + change detection

### Phase 4 — tray UI ✓
- [x] Dynamic menu build from polled state
- [x] PR submenu (Open, Copy, Approve, Snooze)
- [x] Snooze persistence + expiry logic
- [x] Badge count on icon
- [x] Refresh now

### Phase 5 — notifications + updates ✓
- [x] OS notification dispatch (beeep)
- [x] Per-event notification toggle in config
- [x] Update checker (startup + 24h)
- [x] "Update available" menu item

### Phase 6 — distribution ✓
- [x] `.goreleaser.yml` (universal binary, zip)
- [x] Codesign wrapper script (no-op if cert absent)
- [x] Notarization step (env-gated)
- [x] GitHub Actions CI (build + release on tag)
- [x] README (install, OAuth App setup, GHE PAT setup)
