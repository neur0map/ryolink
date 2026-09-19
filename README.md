<div align="center">

# ryolink

The Ryoku store. And a room.

Download the ISO, grab the recovery script, pick up rescue files —
from your terminal, or a browser.

Then stay and talk: a chatroom over SSH where your key is your identity.

</div>

---

<div align="center">

### Connect

```
ssh ryoku.dev
```

</div>

---

### What's inside

**Store** — the landing room. Browse the Ryoku catalog (image, panic-button
recovery, rescue files), copy the canonical download link, `curl` it. The
same catalog serves `https://ryoku.dev/` — landing page, `/api/items` JSON,
and Range-safe file streaming with sha256 shown for every artifact.

**Rooms** — the lounge (where Mika works), gallery, games, suggestions, and
wargame CTF rooms. Chat is what you do after you've picked up your copy.

**Cursor-first** — clicks drive the app: rooms switch, items select, a
second click copies. Keyboard shortcuts keep working beside them;
`Ctrl+M` toggles cursor mode.

**Gallery** — a shared sticky-note board. Post, drag, read what others left
behind.

**Wargames** — OverTheWire CTF rooms with flag submission, leaderboard, and
points.

**GIFs** — search and send animated GIFs inline in chat with `/gif`.

**Mika** — keeper of the bar in the lounge. Tag with `@mika`.

**Music** — optional 24/7 radio streaming.

### Keybinds

```
F1  help        F2  nickname     F3  rooms       F4  mentions
F5  post note   F6  tankard      F7  leaderboard
S   store       ⌃P  commands     ⌃M  cursor mode   `   copy link
```

---

### Run your own

The whole product is **one static binary** — server, storefront, admin CLI,
and all default content (radio catalogue, bar-keeper persona, mystery case,
the store landing page) embedded. No web server required; Caddy is optional
TLS-in-front.

```bash
make ryolink                    # build (CGO off, static)
./ryolink init                  # writes ~/.config/ryolink/ryolink.yaml
./ryolink up                    # run it — then: ssh localhost -p 2222
```

Stock the shelves: point `store.items` at files in the data dir (they stream
with sha256 + resume) or at your mirror URLs (they redirect and count). The
web surface — landing, catalog, downloads — rides the same port as the radio
(`server.web_bind:web_port`, loopback by default; publish it with Caddy).

Production as a service (root, once):

```bash
sudo ./ryolink service install  # generates a hardened systemd unit from your YAML
```

`ryolink.yaml` is the entire control surface: identity, store catalog,
rooms, ports, data dir, input mode, and the **security** block — ryolink
accepts any SSH key by design, so it ships its own in-process firewall
(per-IP connection budgets, auth-fail bans, scanner-probe bans, a persistent
network deny list). `ryolink status` shows config, liveness, and bans at a
glance. Admin commands (`--message`, `--ban`, `--deny`, `--add-room`,
`purge`, …) take effect without a restart; see `ryolink --help`.

**Docker:** `docker compose up` (mount your `ryolink.yaml` at `/data`).

**Environment variables** (all optional; they override the `api:` block):

| Variable | What it does |
|---|---|
| `OPENAI_API_KEY` | Mika / bartender character |
| `KLIPY_API_KEY` | GIF search (`/gif`) |
| `EXA_API_KEY` | Web search for the bar-keeper |
| `REDDIT_CLIENT_ID` / `REDDIT_CLIENT_SECRET` | Reddit feed OAuth |
| `RYOLINK_CONFIG` | Path to ryolink.yaml |
| `RYOLINK_PORT` | Overrides `server.port` |

See [deploy/SETUP.md](deploy/SETUP.md) for the full production runbook
(systemd + port-22 takeover + UFW + fail2ban + optional Caddy).

### Built with

[Bubble Tea](https://github.com/charmbracelet/bubbletea) · [Wish](https://github.com/charmbracelet/wish) · [Lipgloss](https://github.com/charmbracelet/lipgloss) · Go · SQLite

### License

MIT — see [LICENSE](LICENSE)
