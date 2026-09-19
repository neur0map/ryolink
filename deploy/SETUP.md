# Production Setup Guide

Bringing up a `ssh ryoku.dev` ryolink on a blank Ubuntu 24.04 server.
The whole product is ONE static binary: server, admin CLI, and all default
content (radio catalogue, bartender persona, mystery case) are embedded.

Run every command as **root** unless the step says otherwise.

---

## 1. System prerequisites

```bash
apt update && apt upgrade -y
apt install -y ufw fail2ban libcap2-bin curl
```

---

## 2. Create the service user and install the binary

```bash
useradd -m -s /bin/bash ryolink
install -m 0755 -o root -g root ryolink /usr/local/bin/ryolink   # the binary you built
```

No Go toolchain is needed on the server. Build the binary wherever you like
(any machine, `CGO_ENABLED=0 go build ./cmd/ryolink`) and copy it over.

---

## 3. Configure the instance (one YAML)

As the `ryolink` user:

```bash
sudo -u ryolink mkdir -p /home/ryolink/.config/ryolink
sudo -u ryolink ryolink init --domain ryoku.dev --port 22
sudo -u ryolink nano /home/ryolink/.config/ryolink/ryolink.yaml
```

`init` writes a fully-commented config with working defaults for everything:
identity, owner, ports, data dir, idle timeout, web audio, and the whole
**security** block (connection budgets, ban thresholds, deny/allow CIDRs).
The only value it cannot guess is your owner nickname — edit `owner.name`,
and verify `owner.fingerprint` (it auto-detects from `~/.ssh/id_*.pub` if one
exists on this machine; otherwise paste the output of
`ssh-keygen -lf ~/.ssh/id_ed25519.pub` from YOUR laptop).

Everything the instance writes (db, host key, logs, admin signals) lives in
`server.data_dir` — default `/home/ryolink/.local/share/ryolink`. Back that
directory up; `id_ed25519` inside it is ryolink's SSH identity.

> **Canonical config:** `service install` (step 5) stages a copy to
> `/etc/ryolink/ryolink.yaml`, and from then on **that** file is what both
> the server and every `ryolink` admin command read. Edit it (then
> `ryolink service restart`) — re-running `service install` overwrites it
> from whatever config you point `--config` at.

Optional secrets — keep them OUT of the YAML:

```bash
mkdir -p /etc/ryolink
cat > /etc/ryolink/env << 'EOF'
OPENAI_API_KEY=          # bartender
KLIPY_API_KEY=           # /gif search
EXA_API_KEY=             # bartender web search
REDDIT_CLIENT_ID=        # reddit feed (see §7)
REDDIT_CLIENT_SECRET=
EOF
chmod 640 /etc/ryolink/env && chown root:ryolink /etc/ryolink/env
```

(Environment variables win over `api:` values in the config.)

---

## 4. Move the admin SSH off port 22 — BEFORE starting ryolink

Port 22 belongs to ryolink in production; sshd relocates to 2222 and you
lock it to your IPs.

```bash
# EDIT /etc/ssh/sshd_config (or a drop-in in sshd_config.d/):
#   Port 2222
#   PasswordAuthentication no
#   PermitRootLogin no
#   MaxAuthTries 3
#   LoginGraceTime 30
install -d -m 755 /etc/ssh/sshd_config.d
printf 'Port 2222\nPasswordAuthentication no\nPermitRootLogin no\n' \
  > /etc/ssh/sshd_config.d/99-ryolink.conf

# KEEP YOUR CURRENT SESSION OPEN. From your laptop, in a NEW terminal:
ssh -p 2222 root@<VPS_IP>        # must work before you continue
systemctl restart ssh
```

If you connect through a firewall, also allow 2222 from your IP only.

---

## 5. Install the service

```bash
ryolink service install --user ryolink \
  --binary /usr/local/bin/ryolink \
  --config /home/ryolink/.config/ryolink/ryolink.yaml
```

This generates `/etc/systemd/system/ryolink.service` from your config:
non-root user, `CAP_NET_BIND_SERVICE` (the only capability),
`ProtectSystem=strict` with the data dir as the sole writable path,
`PrivateTmp`, `RestrictAddressFamilies`, `UMask=0077`, and reads
`/etc/ryolink/env` if present. It then enables and starts it.

```bash
ryolink status      # config + live banner check + network bans
ssh localhost -p 22 # you should land in the lounge with a ★ next to your name
```

---

## 6. Firewall (UFW)

```bash
ufw default deny incoming
ufw default allow outgoing
ufw allow from <YOUR_IP> to any port 2222 proto tcp   # admin SSH, pinned to you
ufw allow 22/tcp       # ryolink chat
ufw allow 80/tcp       # Caddy (redirects)
ufw allow 443/tcp      # Caddy (landing page)
ufw deny 2222/tcp      # admin SSH to everyone else
ufw enable
ufw status verbose
```

---

## 7. fail2ban for the admin port

ryolink firewalls its OWN port (see the security block in the config:
per-IP budgets, auth-fail bans, scanner-probe bans — all in-process, all
persisted). fail2ban is for the sshd that is left on 2222:

```bash
cat > /etc/fail2ban/jail.d/sshd-admin.conf << 'EOF'
[sshd]
enabled  = true
port     = 2222
maxretry = 3
bantime  = 3600
EOF
systemctl enable --now fail2ban
```

Reddit feed credentials (optional): create a **web app** at
<https://www.reddit.com/prefs/apps/>, redirect URI `http://localhost`, and
put the id/secret in `/etc/ryolink/env`. Then:

```bash
sudo -u ryolink ryolink --feed-add archlinux linuxhyprland
```

---

## 8. Caddy (TLS in front of ryolink's web surface)

There is no static site to deploy anymore: **ryolink serves its own store
landing page, catalog API, downloads, and radio** on one loopback port
(`server.web_bind:server.web_port`, default `127.0.0.1:8090`). Caddy does
TLS and publishes it.

```bash
apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
  | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
  | tee /etc/apt/sources.list.d/caddy-stable.list
apt update && apt install -y caddy

cp deploy/Caddyfile /etc/caddy/Caddyfile
systemctl restart caddy
```

One config change to make first: in `/etc/ryolink/ryolink.yaml` set

```yaml
store:
  public_url: "https://ryoku.dev"   # canonical URL baked into every download link
```

so the storefront, the TUI, and `curl` users all point at the HTTPS front
instead of the raw port. The web surface stays loopback-only; Caddy is the
only thing that can reach :8090.

**Stocking the shelves.** Put artifacts in the data dir (default
`/home/ryolink/.local/share/ryolink`), then reference them in `store.items`
by relative path — the catalog picks up size and sha256 automatically, and
the storefront re-checks disk every time it opens:

```bash
sudo -u ryolink mkdir -p /home/ryolink/.local/share/ryolink/store
sudo -u ryolink cp ~/ryoku-recovery.sh /home/ryolink/.local/share/ryolink/store/
# large files (ISOs) belong on a mirror: give the item a `url:` instead of a
# `path:` and ryolink redirects (and counts) the hop.
```

---
## 9. Verify the deployment

From your laptop:

```bash
bash deploy/verify.sh ryoku.dev
```

Checks the port-22 SSH banner (ryolink), the HTTPS store landing page, TLS
expiry, and the catalog API through Caddy.
---

## Ongoing maintenance

### Live admin (as the ryolink user, no restart needed)

```bash
sudo -u ryolink ryolink --message "brb 5m"       # banner to everyone
sudo -u ryolink ryolink --add-room "linux"
sudo -u ryolink ryolink --ban "grief_nick"       # kicks + blocks the identity
sudo -u ryolink ryolink --deny 198.51.100.0/24   # blocks the NETWORK (live, persistent)
sudo -u ryolink ryolink --deny-list
sudo -u ryolink ryolink purge                    # wipe weekly data (bans survive)
```

### Updating

Replace `/usr/local/bin/ryolink` with a new build (the binary carries its own
assets), then:

```bash
ryolink service restart
```

If you deployed from a git clone instead, `ryolink --update` pulls, rebuilds,
swaps, and restarts in one step.

### Banning etiquette

`--ban` removes a person (their key fingerprint); `--deny` removes a network
(the guard auto-bans the worst offenders on its own — see `ryolink status`).
