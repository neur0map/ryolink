# Contributing

## Local testing

```bash
# Terminal 1 — server (data lands in the config's data_dir)
go run ./cmd/ryolink up

# Terminal 2 — connect via SSH
ssh localhost -p 2222
```

## Branch workflow

```
feature/* ──PR──> dev ──merge──> main (deploy)
```

| Branch | Purpose |
|--------|---------|
| `main` | Production. Runs on the VPS. |
| `dev` | All development. PRs target here. |
| `feature/*` | Short-lived feature branches created from dev. |

1. Create a feature branch from `dev`
2. Open a PR targeting `dev`
3. Test locally
4. When dev is stable, merge to `main` during planned downtime

## Project structure

```
cmd/
  ryolink/           Single binary: SSH server + admin CLI + service manager
internal/
  chat/            Message parsing and storage types
  shop/            Ryoku store front: catalog, HTTP downloads, landing page
  hub/             Connection management, broadcasting
  identity/        Nickname generation, flair, color assignment
  jukebox/         Track catalog, engine, streamer (web audio)
  ratelimit/       Chat rate limiting
  room/            Room definitions
  sanitize/        Input sanitization
  guard/           Application-layer firewall (conn budgets, bans)
  server/          Wish SSH server setup
  session/         Session state, message types
  store/           SQLite persistence
  sudoku/          Multiplayer sudoku game logic
  webstream/       Web audio streaming handler
ui/
  app.go           Main Bubble Tea model
  modal.go         Modal system (help, nick, rooms)
  topbar.go        Top bar with room and stats
  sidebar.go       Rooms panel, online users
  chatview.go      Chat message rendering
  gallery.go       Sticky note board
  sudoku_view.go   Multiplayer sudoku view
  overlay.go       Modal overlay compositor
  styles.go        Ryoku (Tokyo Night) color palette
  splash.go        Welcome screen
  wordmark.go      Block-letter ASCII wordmarks for store logos
  gradient.go      HCL brand gradients
```

Making it look like this: [UI.md](UI.md) — ASCII titles, icons, color, and
the TUI geometry/hit-testing rules.

## Architecture

**Server** — Wish-based SSH server. Each connection gets a Bubble Tea TUI
(cursor-first: clicks drive rooms and store items, keys work beside them).
A shared hub broadcasts messages between sessions. The jukebox engine
manages track playback state for web streaming.

**Store** — `internal/shop` reads `store:` from the config, resolves each
item against the data dir (size + sha256 + resume streaming) or a mirror
URL (302 redirect), and serves it all on the shared web mux with the
landing page. The `#store` room is usually the landing room; the bartender
works the first *chat* room (`cfg.BarRoom()`), not the landing room.

**Web audio** — When `server.web_audio` is true, the server runs an HTTP endpoint (default `127.0.0.1:8090`) serving `/stream` (continuous MP3) and `/now-playing` (JSON metadata). Caddy reverse-proxies these to the public domain; the port itself never faces the internet.

## Setup and admin commands

One binary, two roles: `ryolink up` is the server; every other verb is the
admin CLI talking to the data dir the server watches (live, no restart):

```bash
# Setup / service
ryolink init                                   # write ~/.config/ryolink/ryolink.yaml (store-first)
ryolink up                                     # run the server (foreground)
sudo ryolink service install                   # hardened systemd unit, from your YAML
ryolink status                                 # config, liveness, network bans

# Announcements
ryolink --message "text"                       # Banner to all connected users
ryolink --clear-banner                         # Clear the active banner

# Rooms (live)
ryolink --add-room "name"                      # Add a new room
ryolink --rename-room "old" "new"              # Rename a room
ryolink --remove-room "name"                   # Remove a room (moves users to landing room)

# Moderation
ryolink --ban "nickname"                       # Ban a user by key fingerprint (kicks them)
ryolink --unban "nickname"                     # Unban a user
ryolink --ban-list                             # Show user bans
ryolink --deny <cidr> [reason]                 # Ban a network (live, persisted across restarts)
ryolink --undeny <cidr>                        # Lift a network ban
ryolink --deny-list                            # Show network bans + expiries

# Bartender
ryolink --bartender-off                        # Disable bartender (live)
ryolink --bartender-on                         # Enable bartender (live)

# Reddit feed
ryolink --feed-add sub [sub...]                # Add subreddit(s) to feed
ryolink --feed-remove sub                      # Remove a subreddit from feed
ryolink --feed-list                            # List configured subreddits

# Wargame CTF
ryolink --set-flag bandit 1 "flag"             # Set a wargame flag
ryolink --list-flags bandit                    # List levels with flags

# Data
ryolink purge                                  # Purge all data (bans/owners/drink counts survive)

# Dev deploy (git checkout only)
ryolink --update                               # Pull main, rebuild, swap binary, restart service
```

## Tests

```bash
go test ./...
```
