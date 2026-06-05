# ghnotify

A macOS menubar app that monitors GitHub pull requests — yours and review
requests from others — across github.com and GitHub Enterprise Server instances.

## Features

- **My Pull Requests** — draft, approved, changes-requested, CI status, merge readiness
- **Review Requests** — all open PRs where you're a requested reviewer
- **Grouped by server → repo**, sorted, capped at a configurable limit
- **One-click Approve** directly from the menubar
- **Snooze** a PR until next change or for a fixed duration (1 h / 8 h / 24 h)
- **OS notifications** for review requests, approvals, CI changes (individually configurable)
- **Multiple servers** — github.com + any number of GitHub Enterprise instances
- **Secure credential storage** — macOS Keychain via `go-keyring`
- **Auto-update check** — checks GitHub Releases on startup and every 24 h

## Installation

### Download binary (recommended)

Download the latest `ghnotify_darwin_universal.zip` from the
[Releases page](https://github.com/boyvinall/ghnotify/releases), unzip, and
move the binary somewhere on your `$PATH`:

```sh
unzip ghnotify_darwin_universal.zip
sudo mv ghnotify /usr/local/bin/
```

On first launch macOS may quarantine the binary. Run once to clear it:

```sh
xattr -d com.apple.quarantine /usr/local/bin/ghnotify
```

### Build from source

Requires Go 1.22+ and Xcode Command Line Tools (`xcode-select --install`).

```sh
git clone https://github.com/boyvinall/ghnotify
cd ghnotify
CGO_ENABLED=1 go build -o ghnotify .
```

## First-time setup

Run `ghnotify` — a circle icon appears in your menubar.

Open the menu and click **Add server…** to add GitHub instances.

### github.com (OAuth — recommended)

You need a GitHub OAuth App. Create one at
<https://github.com/settings/developers>:

| Field | Value |
|---|---|
| Application name | ghnotify |
| Homepage URL | `https://github.com/boyvinall/ghnotify` |
| Authorization callback URL | `http://127.0.0.1` (any port; ghnotify uses a random port) |

After creating the app, note the **Client ID** and generate a **Client secret**.

In ghnotify → Add server…:
1. Hostname: `github.com`
2. Auth type: **OAuth**
3. Client ID: *(paste from above)*
4. Client secret: *(paste from above)* — stored in macOS Keychain, never on disk
5. Click **Login** → browser opens → authorize → done

### GitHub Enterprise Server (OAuth)

The same OAuth App flow works for GHE. Register an OAuth App on your GHE
instance at `https://<host>/settings/developers`, then in ghnotify:

1. Hostname: `github.mycompany.com`
2. Auth type: **OAuth**
3. Client ID + secret from your GHE OAuth App

> **GHE version note**: PKCE in the OAuth flow requires GHE ≥ 3.12. Older
> instances will accept the authorization code exchange without verifying the
> PKCE challenge, so it still works — PKCE just adds no extra protection on
> those versions.

### GitHub Enterprise Server (Personal Access Token)

If you can't register an OAuth App on your GHE instance, use a PAT instead:

1. Generate a token at `https://<host>/settings/tokens` with scopes:
   `repo`, `read:org`
2. In ghnotify → Add server…:
   1. Hostname: `github.mycompany.com`
   2. Auth type: **PAT**
3. Enter your token when prompted — stored in macOS Keychain

## Configuration

Config file: `~/Library/Application Support/ghnotify/config.toml`
(created with defaults on first run; open via **Preferences…** in the menu)

```toml
# How often to poll each server for changes.
poll_interval = "60s"

# Maximum PRs shown per section (My PRs / Review Requests).
# Additional PRs are noted as "… and N more".
max_prs_per_section = 20

[notifications]
new_review_requests = true   # a new review request appeared
pr_approved         = true   # one of my PRs was approved
pr_merged           = true   # one of my PRs was closed/merged
ci_status_change    = false  # CI result changed on any tracked PR
new_comments        = false  # comment count changed

# Servers are managed via the menubar (Add server… / Remove server).
# The [[servers]] blocks are written automatically; hand-editing is fine too.

[[servers]]
host      = "github.com"
auth_type = "oauth"
client_id = "Ov23liXXXXXXXXXX"

# [[servers]]
# host      = "github.mycompany.com"
# auth_type = "pat"
```

Changes to `config.toml` take effect on restart.

## Menubar reference

```
[icon]  ← ring = quiet, filled = active

  My Pull Requests  (N)
    [CI][Rev]  server › repo  #NNN  Title
      Open in Browser
      Copy URL
      Approve
      Snooze…
        Until next change
        1 hour
        8 hours
        24 hours

  ─────────────────────────
  Review Requests  (N)
    … (same structure)

  ─────────────────────────
  Servers
    github.com  [Connected as @you]
      Refresh now
      Logout
    github.enterprise.com  [Not connected]
      Login with OAuth…
      Set PAT…
      Remove server

  Add server…
  Preferences…
  Check for updates
  Quit
```

CI glyphs: `✓` passing · `✗` failing · `○` pending · `·` unknown  
Review glyphs: `✓` approved · `✗` changes requested · `○` awaiting · `~` draft

## Secrets required for CI releases

| Secret | Purpose |
|---|---|
| `MACOS_CERTIFICATE` | Base64-encoded `.p12` Developer ID cert |
| `MACOS_CERTIFICATE_PWD` | Password for the `.p12` |
| `CODESIGN_IDENTITY` | e.g. `Developer ID Application: You (TEAMID)` |
| `APPLE_ID` | Apple ID email (for notarytool) |
| `APP_PASSWORD` | App-specific password from appleid.apple.com |
| `TEAM_ID` | 10-character Apple Team ID |

All secrets are optional — the release workflow signs/notarizes when present and
skips gracefully when absent.

To create a release, push a semver tag:

```sh
git tag v1.0.0
git push origin v1.0.0
```

## License

MIT
