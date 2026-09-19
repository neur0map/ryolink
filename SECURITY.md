# ryolink security, in plain language

You are about to connect a program you did not write — a terminal chatroom —
to the internet, using a protocol (SSH) that can do far more than chat. This
document says exactly what ryolink does with that, what it cannot do, and
what you are still responsible for.

## What ryolink is

ryolink is an SSH **server** that speaks exactly one thing: an interactive
terminal session. Your SSH key is your identity — there are no accounts and
no passwords. Everything else SSH could be used for is refused, on purpose,
in code (`internal/server/server.go`):

| SSH feature | ryolink's answer |
|---|---|
| Remote command execution (`ssh host ls`) | refused — exec channels are rejected |
| File transfer (`scp`, `sftp`) | refused — subsystem channels are rejected |
| Local port forwarding (`-L`, pivot/relay) | refused — direct-tcpip channels and forwarding requests are rejected |
| Reverse port forwarding (`-R`) | refused — callbacks return false |
| Agent / X11 forwarding, `env` requests | refused — only `pty-req`, `shell`, `window-change`, `signal`, `break` are accepted inside a session |
| Password / keyboard-interactive auth | refused — public key only |

The library (charmbracelet/wish) restricts channels to `session` only; the
request layer and the auth layer are locked down on top of that. This was
verified with a live attack probe: exec, sftp, scp, `-L` and `-R` all fail
against a running server.

## Abuse defenses (in-process)

ryolink accepts any well-formed SSH key by design — that is the product.
Which means the door is open to scanners and spam too, so it ships its own
firewall (`internal/guard`):

- **Handshake timeout** — a connection that hasn't completed the transport
  handshake in 10 seconds is dropped (slow-loris defense).
- **MaxAuthTries 3** — credential-cycling dies fast, and each rejection
  feeds the ban budget.
- **Auth-failure bans** — repeated failures from an address earn a temporary
  network ban; bans persist in SQLite across restarts.
- **Probe bans** — three short-lived connections from an address that never
  completes a handshake within a minute → ban. This is what port-scanning
  the internet looks like, and it's what most public SSH servers actually
  spend their life handling.
- **Deny list** — config CIDRs + `ryolink --deny` + auto-bans, one list.
- **Per-user** — moderators ban by key fingerprint (`--ban`), not IP.

`ryolink status` shows live ban state.

## What is NOT protected

Read this part twice.

1. **ryolink is not a shell.** Nothing on the machine can be executed
   through it. But if you deploy it badly — same user as other services,
   host key reused from a real SSH server — a bug in Go, the SSH library, or
   ryolink itself is a bug in a network-facing program. Run it as its own
   unprivileged user (the installer does), behind a firewall that only opens
   the port it needs.
2. **Chat content is unencrypted at rest.** The SQLite database holds
   messages in plain text (there is no server-side encryption of chat).
   Weekly purge deletes it; the disk owner can read it before then. Anyone
   with root on the box can read everything. That is true of every chatroom
   of this design; do not put secrets in it.
3. **The weekly purge is the retention policy.** Chat resets every Sunday;
   bans, nick history, and store metadata survive. If you fork ryolink and
   disable the purge, you are now a records-keeper — decide what your
   jurisdiction says about that.
4. **SSH does not hide what you type.** The transport is encrypted, but
   your client's terminal, scrollback, and any keylogger on *your* machine
   are your own responsibility. ryolink cannot protect a compromised client.
5. **Host key trust.** On first connect you will see a host key fingerprint.
   Verify it out-of-band if you care about active MITM on your network path.
   The banner is `SSH-2.0-ryolink` so you can at least tell what answered.
6. **File downloads.** The store serves files with sha256 shown in-app and
   `Range` support. Verify the hash after download — especially the ISO —
   with `sha256sum`. The server showing you a hash is a convenience, not a
   trust anchor; the release page is.

## Reporting

If you find a way through the lockdown above — exec, forwarding, auth
bypass, or a guard bypass — that is a real bug: tell the operator privately
(the owner fingerprint is in `ryolink.yaml`) before telling anyone else.
Do not open a public issue for it.

## For operators

The default config is deliberately locked down; the `security:` block in
`ryolink.yaml` tunes the guard (budgets, ban durations, deny list). The
hardening that actually matters, in order:

1. Dedicated user, no login shell, nothing else running as it.
2. Firewall: only the chat port open; admin SSH on a different port.
3. Keep `store.public_url` pointed at files you actually serve — the
   storefront is honest about what is a mirror vs a direct download.
4. Back up the SQLite file if bans matter to you; everything else is
   disposable by design.
