# DEVELOPMENT.md

The parts of this codebase that will bite you, and what to do instead.
[CONTRIBUTING.md](docs/CONTRIBUTING.md) has the structure and the command
catalogue; this is the folklore. Read it before your first real change.

## It is an SSH server, not a server that happens to speak SSH

The whole app hangs off `charmbracelet/wish`. When a user runs
`ssh ryoku.dev`, your Go process *is* the sshd. That has consequences that
are not obvious from the chat code:

- **Port 22 is a product decision, not an accident.** The reference deploy
  takes over port 22 for chat and moves admin SSH elsewhere. On a box that
  already runs `sshd`, you cannot bind 22 without stopping it — the default
  `ryolink.yaml` uses 2222 for exactly this reason. Don't "fix" the default.
- **The banner lies if you let it.** `wish.WithVersion("ryolink")` produces
  `SSH-2.0-ryolink`. Passing `"SSH-2.0-ryolink"` gives you
  `SSH-2.0-SSH-2.0-ryolink` — the library prepends the prefix itself.
  (This was a real bug; the version string is now correct in
  `internal/server/server.go:103`.)
- **The lockdown lives in three layers**, and a change to one is invisible
  to the others. Channels are restricted to `session` by wish itself; the
  request layer is `SessionRequestCallback` (only pty/shell/window-change/
  signal/break); forwarding is the two `*PortForwardingCallback`s. If you
  add a feature that needs, say, subsystem support, you must touch all
  three or it silently fails at the layer you forgot.

## The guard is not fail2ban

`internal/guard` is in-process, so it sees things fail2ban can't — but it
also only protects ryolink, not the box. Three counters feed bans:
auth failures, connection budget, and **probes** (short-lived connections
that never finish a handshake — `probeTTL`/`probeBanThreshold` in
`guard.go`). Scanners do the third one constantly; that's the signal that
matters most. `RecordAuthFailure` counts *publickey* failures; password
auth is refused before the guard sees it, so don't expect password spam
there.

Bans persist in SQLite. If you wipe the DB you wipe the bans — which is why
`purge` deliberately keeps bans, owners, drink counts, ssh-links, and
bartender memory (each marked `survives purges` in `store.go`).

## UI: Bubble Tea over SSH is not Bubble Tea on your laptop

- **Mouse is opt-in and off by default** (`ctrl+m`). When on, wish emits
  `MouseModeCellMotion`. Clicks carry absolute screen coordinates, so every
  clickable surface needs a `SetOrigin` from the parent to map screen→item.
  The storefront's `itemAtClick` and its `View` must use the *same* row
  geometry — if you change card sizing in one, change it in the other or
  clicks land on the wrong item.
- **Width is in terminal cells, and terminals lie about wide glyphs.** CJK
  (the 力 kanji in store logos) and emoji are 2 cells in most terminals but
  the library counts them as 1 sometimes. Always measure with
  `lipgloss.Width()`, never `len()` or `utf8.RuneCountInString()`. The
  splash's old bug was exactly this: per-line left-pad using `len()` for one
  line and `lipgloss.Width()` for another, so the wordmark drifted off the
  rules. `padLine()` fixes it by making *every* card line exactly `cardW`
  wide — there's a test that asserts it (`ui/splash_test.go`).
- **The splash animates on a tick clock.** Frames advance via
  `splashTickMsg`; if you render the card without driving frames you'll only
  ever see frame 0 (dim banner, first pulse). Tests set `s.frame` directly.
- **Alt-screen + full redraw.** Both splash and app set `v.AltScreen`. There
  is no diffing against a previous frame you can rely on — `View()` must be
  pure and cheap. Don't do DB reads in `View()`.

## Store: catalog is config, files are runtime

`internal/shop` reads `store.items` from config. A `path:` item resolves
against the data dir and gets size+sha256 *lazily, when the storefront
opens* — so you can drop a file in and it appears without a restart. A
`url:` item is a mirror; it 302-redirects and never claims to host the
bytes. The `logo:` field is multi-line YAML ASCII art; if the art has
varying leading whitespace (Arch's does), you need an explicit block
indentation indicator (`|2`), or YAML eats the shapes.

## Testing without a real terminal

`go test -race ./...` covers logic, not rendering. To see the actual UI:

```bash
make run          # builds, starts on :2222, ssh's in
```

For headless render checks, construct the model and call `View()` in a test
— that's how the splash alignment test works. Don't try to drive it over a
PTY in CI; there isn't one.

## Things that look like bugs but are the design

- Any SSH key gets in. That's identity-by-key, not a vuln (see
  [SECURITY.md](SECURITY.md)).
- Chat is gone every Sunday. Purge is a feature.
- The bartender (`@mika`) is silent unless `OPENAI_API_KEY` is set — the
  embedded persona ships, the model doesn't.
- `--update` only works from a git checkout. Packaged installs update via
  the package manager, not the binary pulling its own source.
