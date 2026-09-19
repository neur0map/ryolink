package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
	"ryolink/internal/config"
	"ryolink/internal/guard"
	"ryolink/internal/store"
)

// This file is the operator UX: `ryolink init` writes a complete config,
// `ryolink service install` turns it into a hardened systemd unit, and
// `ryolink status` proves the whole thing is alive. Installing the binary
// and running those three commands is a full setup.

const configTemplate = `# ryolink — one file configures the whole instance.
# Docs: deploy/SETUP.md. Every value has a working default; the only
# required blocks are ryolink + owner.

ryolink:
  name: "%s"
  domain: "%s"
  tagline: ""

owner:
  name: "%s"
  fingerprint: "%s"   # ssh-keygen -lf ~/.ssh/id_ed25519.pub

server:
  host: "0.0.0.0"
  port: %d             # 22 in production (the service unit grants the needed cap)
  data_dir: "%s"   # db, host key, logs, admin signals
  idle_timeout: "2h"       # drop dead sessions
  log_file: ""             # "" = stderr (journald); set a path for a file
  web_audio: false         # 24/7 radio endpoint
  web_bind: "127.0.0.1"    # keep loopback; expose via Caddy, not directly
  web_port: 8090

# The abuse firewall. ryolink accepts any SSH key (the key IS the identity),
# so everything a real sshd guards against is handled here.
security:
  max_conns: 512           # total in-flight connections
  max_conns_per_ip: 4      # one household, four tabs is generous
  new_conns_per_min: 30    # per-IP connection rate
  max_auth_fails: 10       # failed handshakes per minute before a ban
  ban_minutes: 30          # auth-fail ban length
  probe_ban_minutes: 60    # port-scanner ban length
  deny_cidrs: []           # always blocked, e.g. ["203.0.113.0/24"]
  allow_cidrs: []          # when non-empty, ONLY these may connect

# Input mode. ryolink is cursor-first: clicks drive rooms and items; keys
# keep working beside them. ctrl+m toggles live.
bind:
  mouse: "auto"          # auto|on = clicks active, off = keyboard only

# The store front — ryolink's home page and the reason people visit:
# the Ryoku ISO, the panic-button recovery script, rescue files.
# Local items (path, relative to data_dir) stream from this box with
# sha256 + resume; external items (url) redirect to your mirror.
store:
  enabled: true
  title: "Ryoku Store"
  public_url: ""         # e.g. https://dl.ryoku.dev — default: this host
  items:
    - id: "ryoku-iso"
      name: "Ryoku Linux"
      kind: iso
      version: ""
      desc: "The live image. Arch underneath, Ryoku desktop on top."
      url: "https://github.com/ryoku-dev/ryoku-arch/releases/latest"
      logo: "ARCH"           # one word = block-letter wordmark; multi-line = ASCII art
    - id: "recovery"
      name: "ryoku-recovery"
      kind: script
      desc: "The panic button: rebuild the desktop when an update bites."
      path: "store/ryoku-recovery.sh"
      logo: "RYOKU"

# The data wipe that keeps the room disposable (UTC).
purge:
  disabled: false
  weekday: "sunday"
  time: "23:59"

# The reddit feed catalog — this list is the source of truth; edit it and
# "ryolink service reload". Needs reddit keys below to actually fetch.
feed:
  subreddits:
    - "archlinux"
    - "unixporn"

# Wargame CTF boards. Add a room of type wargame (see rooms: below) and
# list its flags here: level -> password. An empty value removes a level.
# Synced at startup and on "ryolink service reload".
wargame:
  games: {}
  #   bandit:
  #     1: "the-password-for-level-1"
  #     2: ""        # removes level 2


# Optional API keys. Environment variables of the same name win, so secrets
# can stay out of this file (EnvironmentFile= in systemd).
api:
  openai_key: ""           # bartender (OPENAI_API_KEY)
  klipy_key: ""            # /gif (KLIPY_API_KEY)
  exa_key: ""              # bartender web search (EXA_API_KEY)
  reddit_client_id: ""     # reddit feed OAuth (REDDIT_CLIENT_ID)
  reddit_secret: ""        # (REDDIT_CLIENT_SECRET)

rooms:
  # First room = landing = the store front; chat rooms are what you
  # wander into after.
  - name: "store"
    type: store
  - name: "lounge"
    type: chat
  - name: "gallery"
    type: gallery
  - name: "games"
    type: games
`

// runInit writes a fresh config (and the data dir) and exits. Flags:
//
//	ryolink init [--domain D] [--name N] [--port P] [--force]
func runInit(args []string) {
	domain := "ryoku.dev"
	name := "ryolink"
	port := 2222
	force := false
	user := "owner"
	if u := os.Getenv("USER"); u != "" {
		user = u
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--domain":
			if i+1 < len(args) {
				domain = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--port":
			if i+1 < len(args) {
				fmt.Sscan(args[i+1], &port)
				i++
			}
		case "--force":
			force = true
		}
	}

	dir := filepath.Join(os.Getenv("HOME"), ".config", "ryolink")
	if c := os.Getenv("XDG_CONFIG_HOME"); c != "" {
		dir = filepath.Join(c, "ryolink")
	}
	cfgPath := filepath.Join(dir, "ryolink.yaml")
	dd := filepath.Join(os.Getenv("HOME"), ".local", "share", "ryolink")
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		dd = filepath.Join(d, "ryolink")
	}
	if _, err := os.Stat(cfgPath); err == nil && !force {
		fmt.Printf("Config already exists: %s\n(re-run with --force to overwrite)\n", cfgPath)
		os.Exit(1)
	}
	fp := detectOwnerFingerprint()
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dd, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}
	body := fmt.Sprintf(configTemplate, name, domain, user, fp, port, dd)
	if err := yaml.Unmarshal([]byte(body), new(any)); err != nil {
		fmt.Fprintf(os.Stderr, "init: template produced invalid yaml (bug): %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}
	// prove it parses before telling the user it's ready
	if _, err := config.Load(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "init: generated config failed validation: %v\n", err)
		os.Exit(1)
	}
	// create + migrate the database now, so `up` and `service install`
	// never race a half-built store
	if st, err := store.New(filepath.Join(dd, "ryolink.db")); err != nil {
		fmt.Fprintf(os.Stderr, "init: database: %v\n", err)
		os.Exit(1)
	} else {
		st.Close()
	}
	fmt.Printf("Config written: %s\nData dir:       %s\nOwner key:      %s\n\n", cfgPath, dd, fp)
	fmt.Println("Start it:  ryolink up")
	fmt.Println("As a service (root):  sudo ryolink service install")
}

// detectOwnerFingerprint picks the first plausible public key in ~/.ssh and
// reports its SHA256 fingerprint (the format the config accepts).
func detectOwnerFingerprint() string {
	home := os.Getenv("HOME")
	cands, _ := filepath.Glob(filepath.Join(home, ".ssh", "id_*.pub"))
	if len(cands) == 0 {
		cands, _ = filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
	}
	for _, c := range cands {
		out, err := exec.Command("ssh-keygen", "-lf", c).Output()
		if err == nil {
			parts := strings.Fields(strings.TrimSpace(string(out)))
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return "SHA256:paste-your-ssh-key-fingerprint"
}

// serviceUnit renders the production systemd unit for an installed instance.
// dataDir is both the working directory and the single writable path;
// configPath is the staged copy inside it.
func serviceUnit(user, binary, dataDir, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=ryolink chatroom
Documentation=https://ryoku.dev
After=network.target

[Service]
Type=simple
User=%[1]s
WorkingDirectory=%[3]s
Environment=RYOLINK_CONFIG=%[4]s
EnvironmentFile=-/etc/ryolink/env
ExecStart=%[2]s up
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=3
LimitNOFILE=65535

# production ports (22) without root: the service gets exactly this one cap
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

NoNewPrivileges=yes
PrivateTmp=yes
PrivateDevices=yes
ProtectControlGroups=yes
ProtectKernelModules=yes
ProtectKernelTunables=yes
ProtectClock=yes
RestrictSUIDSGID=yes
LockPersonality=yes
RestrictNamespaces=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%[3]s
UMask=0077

[Install]
WantedBy=multi-user.target
`, user, binary, dataDir, configPath)
}

// runService: ryolink service install|remove|start|stop|restart|reload|status|logs
func runService(args []string) {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "install":
		installService(args[1:])
	case "remove":
		mustRoot()
		run("systemctl", "disable", "--now", "ryolink")
		os.Remove("/etc/systemd/system/ryolink.service")
		run("systemctl", "daemon-reload")
		fmt.Println("ryolink.service removed (data dir and config untouched).")
	case "start", "stop", "restart", "reload", "status":
		run("systemctl", sub, "ryolink")
	case "logs":
		cmd := exec.Command("journalctl", "-u", "ryolink", "--no-pager", "-n", "100")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	default:
		fmt.Println("Usage: ryolink service install|remove|start|stop|restart|reload|status|logs")
	}
}

func mustRoot() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "this command needs root: sudo ryolink ...")
		os.Exit(1)
	}
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s: %v\n", name, strings.Join(args, " "), err)
	}
}

// installService writes the hardened unit from the current config. Runs as
// root (sudo). Flags: --user NAME (service account), --binary PATH
// (ExecStart), --config PATH (which ryolink.yaml to install from). The
// config's server.data_dir becomes the service's only writable path.
func installService(args []string) {
	mustRoot()
	svcUser := ""
	binary := ""
	explicitCfg := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 < len(args) {
				svcUser = args[i+1]
				i++
			}
		case "--binary":
			if i+1 < len(args) {
				binary = args[i+1]
				i++
			}
		case "--config":
			if i+1 < len(args) {
				explicitCfg = args[i+1]
				i++
			}
		}
	}

	cfgPath := explicitCfg
	if cfgPath == "" {
		cfgPath = findConfigSystemWide()
	}
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "no ryolink.yaml found — run `ryolink init` first,")
		fmt.Fprintln(os.Stderr, "or set RYOLINK_CONFIG=/path/to/ryolink.yaml")
		os.Exit(1)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config invalid: %v\n", err)
		os.Exit(1)
	}
	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "locate binary: %v\n", err)
			os.Exit(1)
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		binary = exe
	}

	user, uid := resolveServiceUser(svcUser, cfg.Server.DataDir)
	if uid < 0 {
		name := svcUser
		if name == "" {
			name = "ryolink"
		}
		fmt.Fprintf(os.Stderr, "service user %q does not exist — create it first:\n  useradd -m -s /bin/bash %s\n", name, name)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "data dir: %v\n", err)
		os.Exit(1)
	}
	chown(cfg.Server.DataDir, uid)
	// Stage the config where root owns it and the service user can read it:
	// /etc/ryolink/ryolink.yaml (findConfigSystemWide looks there first).
	// It cannot live inside the data dir — that is 0700 to the service user,
	// so root could not have written it there in the first place.
	sysDir := "/etc/ryolink"
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "config dir: %v\n", err)
		os.Exit(1)
	}
	sysCfg := filepath.Join(sysDir, "ryolink.yaml")
	b, _ := os.ReadFile(cfgPath)
	if err := os.WriteFile(sysCfg, b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "stage config: %v\n", err)
		os.Exit(1)
	}

	unitPath := "/etc/systemd/system/ryolink.service"
	if err := os.WriteFile(unitPath, []byte(serviceUnit(user, binary, cfg.Server.DataDir, sysCfg)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write unit: %v\n", err)
		os.Exit(1)
	}
	run("systemctl", "daemon-reload")
	// enable + restart, not `enable --now`: a re-install (new binary or new
	// config) must actually take effect on a unit that is already running.
	run("systemctl", "enable", "ryolink")
	run("systemctl", "restart", "ryolink")
	fmt.Printf("\nryolink service installed: user=%s port=%d data=%s\n", user, cfg.Server.Port, cfg.Server.DataDir)
	fmt.Printf("Check:  ryolink status\nTest:   ssh localhost -p %d\n", cfg.Server.Port)
	if cfg.Server.Port == 22 {
		fmt.Println("NOTE: port 22 — move your real sshd off 22 first (deploy/SETUP.md §4).")
	}
}

// resolveServiceUser: --user flag, else the owner of the data dir (that is
// whoever's instance this is), else the conventional ryolink account.
// Returns uid < 0 when nothing resolves.
func resolveServiceUser(flag, dataDir string) (string, int) {
	if flag != "" {
		return lookupUser(flag)
	}
	if fi, err := os.Stat(dataDir); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			return lookupUser(strconv.Itoa(int(st.Uid)))
		}
	}
	return lookupUser("ryolink")
}

func lookupUser(nameOrUID string) (string, int) {
	u, err := user.Lookup(nameOrUID)
	if err != nil {
		if uid, cerr := strconv.Atoi(nameOrUID); cerr == nil {
			u, err = user.LookupId(strconv.Itoa(uid))
			if err != nil {
				return "", -1
			}
		} else {
			return "", -1
		}
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return "", -1
	}
	return u.Username, uid
}

// findConfigSystemWide: /etc/ryolink/ryolink.yaml first, then the user rules.
func findConfigSystemWide() string {
	if _, err := os.Stat("/etc/ryolink/ryolink.yaml"); err == nil {
		return "/etc/ryolink/ryolink.yaml"
	}
	return findConfig()
}

func chown(path string, uid int) {
	if uid < 0 {
		return
	}
	exec.Command("chown", "-R", fmt.Sprintf("%d:", uid), path).Run()
}

// runUp starts the server in the foreground (the historical default mode,
// kept as an explicit verb so `ryolink` alone is never ambiguous).
func runUp() {
	runServer()
}

// runStatus reports the whole instance: config, liveness, bans.
func runStatus() {
	cfg := loadConfigForAdmin()
	if cfg == nil {
		fmt.Println("No valid ryolink.yaml found. Run: ryolink init")
		return
	}
	fmt.Printf("ryolink %s — %s:%d\n", cfg.Ryolink.Name, cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("config:    %s\n", findConfig())
	fmt.Printf("data dir:  %s\n", dataDir)
	fmt.Printf("rooms:     %s\n", strings.Join(cfg.RoomNames(), ", "))
	fmt.Printf("security:  max_conns=%d per_ip=%d rate=%d/min auth_ban=%dm probe_ban=%dm\n",
		cfg.Security.MaxConns, cfg.Security.MaxConnsPerIP, cfg.Security.NewConnsPerMin,
		cfg.Security.BanMinutes, cfg.Security.ProbeBanMinutes)
	if len(cfg.Security.DenyCIDRs) > 0 {
		fmt.Printf("deny:      %s\n", strings.Join(cfg.Security.DenyCIDRs, " "))
	}
	if len(cfg.Security.AllowCIDRs) > 0 {
		fmt.Printf("allow:     %s (ONLY these)\n", strings.Join(cfg.Security.AllowCIDRs, " "))
	}

	// live check: TCP-dial the port and read the SSH banner
	dialHost := cfg.Server.Host
	if dialHost == "" || dialHost == "0.0.0.0" || dialHost == "::" {
		dialHost = "127.0.0.1"
	}
	addr := net.JoinHostPort(dialHost, fmt.Sprint(cfg.Server.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		fmt.Printf("server:    DOWN (%v)\n", err)
	} else {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		b := make([]byte, 40)
		n, _ := conn.Read(b)
		conn.Close()
		fmt.Printf("server:    UP — %s\n", strings.TrimSpace(string(b[:n])))
	}

	// persistent network bans
	st, err := store.New(resolvedDBPath())
	if err == nil {
		denies := st.ListDenies(time.Now())
		st.Close()
		if len(denies) > 0 {
			fmt.Printf("net bans:  %d active (lift: ryolink --undeny <cidr>)\n", len(denies))
			for _, d := range denies[:min(5, len(denies))] {
				exp := "permanent"
				if !d.Until.IsZero() {
					exp = d.Until.UTC().Format(time.RFC3339)
				}
				fmt.Printf("  %-22s %-20s %s\n", d.CIDR, exp, d.Reason)
			}
		} else {
			fmt.Println("net bans:  none")
		}
	}
}

// runDenyList / runDeny / runUndeny manage the persistent IP deny list.
func runDenyList() {
	loadConfigForAdmin()
	st, err := store.New(resolvedDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	denies := st.ListDenies(time.Now())
	if len(denies) == 0 {
		fmt.Println("No network bans.")
		return
	}
	fmt.Printf("%-24s %-22s %s\n", "CIDR", "EXPIRES", "REASON")
	for _, d := range denies {
		exp := "permanent"
		if !d.Until.IsZero() {
			exp = d.Until.UTC().Format(time.RFC3339)
		}
		fmt.Printf("%-24s %-22s %s\n", d.CIDR, exp, d.Reason)
	}
}

func runDeny(cidr, reason string) {
	loadConfigForAdmin()
	if err := guard.ValidCIDR(cidr); err != nil {
		fmt.Fprintln(os.Stderr, "bad CIDR/IP:", cidr)
		os.Exit(1)
	}
	st, err := store.New(resolvedDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	perm := time.Time{}
	if err := st.AddDeny(cidr, &perm, reason); err != nil {
		fmt.Fprintf(os.Stderr, "deny: %v\n", err)
		os.Exit(1)
	}
	// live: the running server watches the deny signal file and re-loads bans
	_ = os.WriteFile(dataPath(denyFile), []byte("deny:"+cidr), 0600)
	fmt.Printf("Denied %s (live).\n", cidr)
}

func runUndeny(cidr string) {
	loadConfigForAdmin()
	st, err := store.New(resolvedDBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.RemoveDeny(cidr); err != nil {
		fmt.Fprintf(os.Stderr, "unban: %v\n", err)
		os.Exit(1)
	}
	_ = os.WriteFile(dataPath(denyFile), []byte("undeny:"+cidr), 0600)
	fmt.Printf("Lifted network ban on %s.\n", cidr)
}
