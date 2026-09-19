package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"ryolink/internal/bartender"
	"ryolink/internal/config"
	"ryolink/internal/dm"
	"ryolink/internal/gif"
	"ryolink/internal/guard"
	"ryolink/internal/hub"
	"ryolink/internal/jukebox"
	"ryolink/internal/mystery"
	"ryolink/internal/poll"
	"ryolink/internal/reddit"
	"ryolink/internal/sanitize"
	"ryolink/internal/search"
	"ryolink/internal/server"
	"ryolink/internal/session"
	"ryolink/internal/shop"
	"ryolink/internal/store"
	"ryolink/internal/sudoku"
	"ryolink/internal/version"
	"ryolink/internal/wargame"
	"ryolink/internal/webstream"
)

const bannerFile = ".banner"
const bartenderToggleFile = ".bartender-toggle"
const addRoomFile = ".addroom"
const renameRoomFile = ".renameroom"
const removeRoomFile = ".removeroom"
const banFile = ".ban"
const purgeFile = ".purge"
const denyFile = ".deny"

func main() {
	// `version` answers without touching any config — the release
	// workflow reads its notes through the same parser the TUI uses.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		if len(os.Args) > 2 && os.Args[2] == "--notes" {
			fmt.Println(version.Notes())
		} else {
			fmt.Println(version.Version)
		}
		return
	}
	// `init` must run before config resolution (it creates the config).
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInit(os.Args[2:])
		return
	}
	// Resolve the data dir (and thus every runtime path) before dispatching
	// any verb, so admin signal files land where the server watches them.
	loadConfigForAdmin()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "up":
			runUp()
			return
		case "service":
			runService(os.Args[2:])
			return
		case "status":
			runStatus()
			return
		case "purge":
			runPurge()
			return
		case "--message":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --message \"your message here\"")
				os.Exit(1)
			}
			runMessage(os.Args[2])
			return
		case "--add-room":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --add-room \"room_name\"")
				os.Exit(1)
			}
			runAddRoom(os.Args[2])
			return
		case "--rename-room":
			if len(os.Args) < 4 {
				fmt.Println("Usage: ryolink --rename-room \"old_name\" \"new_name\"")
				os.Exit(1)
			}
			runRenameRoom(os.Args[2], os.Args[3])
			return
		case "--remove-room":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --remove-room \"room_name\"")
				os.Exit(1)
			}
			runRemoveRoom(os.Args[2])
			return
		case "--ban":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --ban \"nickname\"")
				os.Exit(1)
			}
			runBan(os.Args[2])
			return
		case "--unban":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --unban \"nickname\"")
				os.Exit(1)
			}
			runUnban(os.Args[2])
			return
		case "--ban-list":
			runBanList()
			return
		case "--deny":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --deny <cidr|ip> [reason]")
				os.Exit(1)
			}
			reason := "banned by admin"
			if len(os.Args) > 3 {
				reason = strings.Join(os.Args[3:], " ")
			}
			runDeny(os.Args[2], reason)
			return
		case "--undeny":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --undeny <cidr|ip>")
				os.Exit(1)
			}
			runUndeny(os.Args[2])
			return
		case "--deny-list":
			runDenyList()
			return
		case "--clear-banner":
			runClearBanner()
			return
		case "--set-flag":
			if len(os.Args) < 5 {
				fmt.Println("Usage: ryolink --set-flag <wargame> <level> <flag>")
				os.Exit(1)
			}
			runSetFlag(os.Args[2], os.Args[3], os.Args[4])
			return
		case "--list-flags":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --list-flags <wargame>")
				os.Exit(1)
			}
			runListFlags(os.Args[2])
			return
		case "--bartender-off":
			os.WriteFile(dataPath(bartenderToggleFile), []byte("off"), 0600)
			fmt.Println("Bartender disable signal sent.")
			return
		case "--bartender-on":
			os.WriteFile(dataPath(bartenderToggleFile), []byte("on"), 0600)
			fmt.Println("Bartender enable signal sent.")
			return
		case "--update":
			if err := runUpdate(); err != nil {
				log.Fatalf("update: %v", err)
			}
			return
		case "--feed-add":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --feed-add <subreddit> [subreddit...]")
				os.Exit(1)
			}
			runFeedAdd(os.Args[2:])
			return
		case "--feed-remove":
			if len(os.Args) < 3 {
				fmt.Println("Usage: ryolink --feed-remove <subreddit>")
				os.Exit(1)
			}
			runFeedRemove(os.Args[2])
			return
		case "--feed-list":
			runFeedList()
			return
		case "help", "--help", "-h":
			printUsage()
			return
		default:
			fmt.Printf("unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	// bare `ryolink` starts the server (the systemd unit runs `ryolink up`;
	// both work). This is the same behaviour as before the rework.
	runServer()
}

func printUsage() {
	fmt.Println("ryolink — a chatroom over SSH. Single binary: server + admin CLI.")
	fmt.Println()
	fmt.Println("Setup:")
	fmt.Println("  ryolink init [--domain D] [--port P]     Write a starter config (~/.config/ryolink)")
	fmt.Println("  ryolink up                               Run the server in the foreground")
	fmt.Println("  sudo ryolink service install             Install + start the hardened systemd unit")
	fmt.Println("  ryolink status                           Config, liveness, bans at a glance")
	fmt.Println()
	fmt.Println("Admin (live, via the data dir signal files):")
	fmt.Println("  ryolink --message \"text\"                 Banner to all connected users")
	fmt.Println("  ryolink --clear-banner                   Clear the banner")
	fmt.Println("  ryolink --add-room \"name\"                Add a room (no restart)")
	fmt.Println("  ryolink --rename-room \"old\" \"new\"        Rename a room")
	fmt.Println("  ryolink --remove-room \"name\"             Remove a room")
	fmt.Println("  ryolink --ban \"nickname\"                 Ban a user by nickname (kicks them)")
	fmt.Println("  ryolink --unban \"nickname\"               Unban a user")
	fmt.Println("  ryolink --ban-list                       Show user bans")
	fmt.Println("  ryolink --deny <cidr> [reason]           Ban a network (live, persistent)")
	fmt.Println("  ryolink --undeny <cidr>                  Lift a network ban")
	fmt.Println("  ryolink --deny-list                      Show network bans")
	fmt.Println("  ryolink --bartender-off / --bartender-on Toggle the bartender (live)")
	fmt.Println("  ryolink --set-flag <game> <lvl> \"flag\"   Set a wargame flag (or edit wargame: in ryolink.yaml)")
	fmt.Println("  ryolink --list-flags <game>              List wargame flags")
	fmt.Println("  ryolink --feed-add <sub> [sub...]        Add subreddit(s) to the feed (or edit feed: in ryolink.yaml)")
	fmt.Println("  ryolink --feed-remove <sub>              Remove a subreddit")
	fmt.Println("  ryolink --feed-list                      List feed subreddits")
	fmt.Println("  ryolink purge                            Purge weekly data (bans survive)")
	fmt.Println("  ryolink --update                         Git dev deploy: pull, rebuild, restart")
}

func runMessage(text string) {
	if err := os.WriteFile(dataPath(bannerFile), []byte(text), 0600); err != nil {
		log.Fatalf("failed to write banner: %v", err)
	}
	fmt.Printf("Banner sent: %s\n", text)
}

func runClearBanner() {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	st.ClearBanner()
	fmt.Println("Banner cleared.")
}

func runAddRoom(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		fmt.Println("Room name cannot be empty.")
		os.Exit(1)
	}
	if err := os.WriteFile(dataPath(addRoomFile), []byte(name), 0600); err != nil {
		log.Fatalf("failed to write addroom file: %v", err)
	}
	fmt.Printf("Room queued: #%s (will appear when server picks it up)\n", name)
}

func runRenameRoom(oldName, newName string) {
	oldName = strings.ToLower(strings.TrimSpace(oldName))
	newName = strings.ToLower(strings.TrimSpace(newName))
	if oldName == "" || newName == "" {
		fmt.Println("Room names cannot be empty.")
		os.Exit(1)
	}
	// Format: "old:new"
	payload := oldName + ":" + newName
	if err := os.WriteFile(dataPath(renameRoomFile), []byte(payload), 0600); err != nil {
		log.Fatalf("failed to write rename file: %v", err)
	}
	fmt.Printf("Rename queued: #%s → #%s (will apply when server picks it up)\n", oldName, newName)
}

func runRemoveRoom(name string) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		fmt.Println("Room name cannot be empty.")
		os.Exit(1)
	}
	// Protect the landing room
	firstRoom := "lounge" // fallback
	if cfg, err := config.Load(findConfig()); err == nil {
		firstRoom = cfg.FirstRoom()
	}
	if name == firstRoom {
		fmt.Printf("Cannot remove the landing room #%s\n", name)
		os.Exit(1)
	}
	if err := os.WriteFile(dataPath(removeRoomFile), []byte(name), 0600); err != nil {
		log.Fatalf("failed to write remove file: %v", err)
	}
	fmt.Printf("Remove queued: #%s (users will be moved to #%s)\n", name, firstRoom)
}

func runBan(nickname string) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		fmt.Println("Nickname cannot be empty.")
		os.Exit(1)
	}
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	fp, err := st.FingerprintByNickname(nickname)
	if err != nil {
		fmt.Printf("User %q not found.\n", nickname)
		os.Exit(1)
	}

	if err := st.Ban(fp, "banned by admin", nil); err != nil {
		log.Fatalf("ban failed: %v", err)
	}

	// Signal the server to kick them
	if err := os.WriteFile(dataPath(banFile), []byte(fp), 0600); err != nil {
		log.Fatalf("failed to write ban file: %v", err)
	}

	fmt.Printf("Banned %s (fingerprint: %s). They will be kicked if online.\n", nickname, fp[:16]+"...")
}

func runUnban(nickname string) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		fmt.Println("Nickname cannot be empty.")
		os.Exit(1)
	}
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	fp, err := st.FingerprintByNickname(nickname)
	if err != nil {
		fmt.Printf("User %q not found.\n", nickname)
		os.Exit(1)
	}

	if err := st.Unban(fp); err != nil {
		log.Fatalf("unban failed: %v", err)
	}
	fmt.Printf("Unbanned %s.\n", nickname)
}

func runBanList() {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	bans, err := st.BanList()
	if err != nil {
		log.Fatalf("failed to list bans: %v", err)
	}
	if len(bans) == 0 {
		fmt.Println("No active bans.")
		return
	}

	fmt.Printf("%-20s %-20s %s\n", "NICKNAME", "BANNED AT", "FINGERPRINT")
	fmt.Println(strings.Repeat("─", 64))
	for _, b := range bans {
		fp := b.Fingerprint
		if len(fp) > 16 {
			fp = fp[:16] + "..."
		}
		fmt.Printf("%-20s %-20s %s\n", b.Nickname, b.BannedAt, fp)
	}
	fmt.Printf("\n%d ban(s) total.\n", len(bans))
}

func runSetFlag(wargameName, levelStr, flag string) {
	level, err := strconv.Atoi(levelStr)
	if err != nil || level < 1 {
		fmt.Println("Level must be a positive number.")
		os.Exit(1)
	}
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	ws := wargame.New(st.DB())
	if err := ws.SetFlag(wargameName, level, flag); err != nil {
		log.Fatalf("set flag: %v", err)
	}
	fmt.Printf("Flag set: %s level %d\n", wargameName, level)
}

func runListFlags(wargameName string) {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	ws := wargame.New(st.DB())
	levels := ws.ListFlags(wargameName)
	if len(levels) == 0 {
		fmt.Printf("No flags set for %s\n", wargameName)
		return
	}
	fmt.Printf("%s flags (%d levels):\n", wargameName, len(levels))
	for _, l := range levels {
		fmt.Printf("  level %d  ✓\n", l)
	}
}

func runFeedAdd(subreddits []string) {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	for _, sub := range subreddits {
		sub = strings.ToLower(strings.TrimSpace(sub))
		sub = strings.TrimPrefix(sub, "r/")
		if sub == "" {
			continue
		}
		if err := st.AddFeedSubreddit(sub, "admin"); err != nil {
			fmt.Printf("Failed to add r/%s: %v\n", sub, err)
			continue
		}
		fmt.Printf("Added r/%s to feed\n", sub)
	}
}

func runFeedRemove(subreddit string) {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	subreddit = strings.ToLower(strings.TrimSpace(subreddit))
	subreddit = strings.TrimPrefix(subreddit, "r/")
	if err := st.RemoveFeedSubreddit(subreddit); err != nil {
		log.Fatalf("remove failed: %v", err)
	}
	fmt.Printf("Removed r/%s from feed\n", subreddit)
}

func runFeedList() {
	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	subs := st.FeedSubreddits()
	if len(subs) == 0 {
		fmt.Println("No subreddits configured. Use --feed-add to add some.")
		return
	}
	fmt.Println("Feed subreddits:")
	for _, sub := range subs {
		fmt.Printf("  r/%s\n", sub)
	}
}

func runPurge() {
	// Broadcast purge to connected clients before wiping
	os.WriteFile(dataPath(purgeFile), []byte("1"), 0600)
	fmt.Println("Signaled connected clients...")
	time.Sleep(2 * time.Second) // give server time to broadcast

	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	fmt.Println("Purging all data (users, chat, gallery, visitors)...")
	fmt.Println("Bans and owners preserved.")
	if err := st.PurgeAll(); err != nil {
		log.Fatalf("purge failed: %v", err)
	}
	st.Close()

	fmt.Println("Restarting server...")
	cmd := exec.Command("sudo", "/usr/bin/systemctl", "restart", "ryolink")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("restart failed: %v (restart manually with: sudo systemctl restart ryolink)", err)
	} else {
		fmt.Println("Done. Server restarted with clean state.")
	}
}

func runServer() {
	// config: explicit env, cwd, or the user/system config dir
	configPath := findConfig()
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "ERROR: no ryolink.yaml found.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Configure your instance in one step:")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  ryolink init                # writes ~/.config/ryolink/ryolink.yaml")
		fmt.Fprintln(os.Stderr, "  ryolink up                  # run it")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Or drop a ryolink.yaml next to this binary / in the working directory.")
		fmt.Fprintln(os.Stderr, "Your owner fingerprint: ssh-keygen -lf ~/.ssh/id_ed25519.pub")
		os.Exit(1)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	resolveDataDir(cfg, configPath)

	// logging: a configured file, or stderr (journald captures it in service
	// mode). The old behaviour (ryolink.log in the data dir) is the template
	// default for non-service runs.
	if cfg.Server.LogFile != "" {
		logFile, err := os.OpenFile(cfg.Server.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
	}

	sanitize.SetOwnerNick(cfg.Owner.Name)

	st, err := store.New(resolvedDBPath())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	st.SeedRooms(cfg.RoomNames())

	// config is the source of truth for the wargame board and the feed:
	// edit ryolink.yaml, restart (or reload), done.
	if len(cfg.Wargame.Games) > 0 {
		if n, err := wargame.New(st.DB()).Sync(cfg.Wargame.Games); err != nil {
			log.Printf("wargame: flag sync: %v", err)
		} else if n > 0 {
			log.Printf("wargame: synced %d flag changes from config", n)
		}
	}
	if len(cfg.Feed.Subreddits) > 0 {
		if err := st.SyncFeedSubreddits(cfg.Feed.Subreddits); err != nil {
			log.Printf("feed: sync: %v", err)
		}
	}

	h := hub.New()
	go h.Run()

	// host key lives with the instance data; wish generates it on first start.
	// Back this file up: losing it changes ryolink's SSH identity for every
	// returning user.
	hostKeyDir := filepath.Join(dataDir, ".ssh")
	if err := os.MkdirAll(hostKeyDir, 0700); err != nil {
		log.Fatalf("host key dir: %v", err)
	}

	// abuse firewall
	idleTimeout := 2 * time.Hour
	if d, err := time.ParseDuration(cfg.Server.IdleTimeout); err == nil {
		idleTimeout = d
	}
	g := guard.New(guard.Policy{
		MaxConns:       cfg.Security.MaxConns,
		MaxConnsPerIP:  cfg.Security.MaxConnsPerIP,
		NewConnsPerMin: cfg.Security.NewConnsPerMin,
		MaxAuthFails:   cfg.Security.MaxAuthFails,
		BanDuration:    time.Duration(cfg.Security.BanMinutes) * time.Minute,
		ProbeBan:       time.Duration(cfg.Security.ProbeBanMinutes) * time.Minute,
		DenyCIDRs:      cfg.Security.DenyCIDRs,
		AllowCIDRs:     cfg.Security.AllowCIDRs,
	}, st)

	catalog := jukebox.NewCatalog()
	log.Printf("Ryolink Radio: %d tracks loaded", catalog.TrackCount())
	jukeboxEngine := jukebox.NewEngineWithCatalog(catalog)
	jukeboxEngine.SetOnlineCount(h.OnlineCount)
	streamer := jukebox.NewStreamer()
	streamer.SetOnDurationKnown(func(seconds int) {
		jukeboxEngine.UpdateDuration(seconds)
	})
	streamer.SetOnError(func() {
		// Download failed — immediately retry with a new track
		jukeboxEngine.RetryTrack()
	})
	jukeboxEngine.SetOnTrackChange(func(track jukebox.Track) {
		streamer.StreamTrack(track)
	})

	sudokuGame := sudoku.NewGame("evil")
	log.Printf("Sudoku: evil puzzle ready (%d clues)", sudokuGame.Filled())

	// One web surface on server.web_bind:web_port — the store landing page
	// (when enabled), the shop JSON + download endpoints, and the radio
	// stream. It defaults to loopback; Caddy publishes it. The storefront
	// registers "/" only when enabled, so a chat-only instance answers
	// /stream and /now-playing exactly as before.
	webMux := http.NewServeMux()
	var shopInst *shop.Shop
	if cfg.Store.Enabled {
		publicBase := cfg.Store.PublicURL
		if publicBase == "" {
			publicBase = "http://" + cfg.Ryolink.Domain + ":" + strconv.Itoa(cfg.Server.WebPort)
		}
		sh, err := shop.New(cfg.Store, dataDir, publicBase, st.DB())
		if err != nil {
			log.Printf("shop: disabled (%v)", err)
		} else {
			shopInst = sh
			sh.RegisterRoutes(webMux, true)
			log.Printf("shop: %d items, landing on :%d", len(sh.Items()), cfg.Server.WebPort)
		}
	}
	ws := webstream.New(streamer, jukeboxEngine)
	ws.RegisterRoutes(webMux)
	if shopInst == nil {
		// store disabled: the web port still answers with a minimal ryoku page
		webMux.HandleFunc("/", func(w2 http.ResponseWriter, r2 *http.Request) {
			if r2.URL.Path != "/" {
				http.NotFound(w2, r2)
				return
			}
			w2.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w2, "<!doctype html><meta charset=utf-8><title>%s</title>"+
				"<body style=\"background:#16161e;color:#c0caf5;font:15px ui-monospace,monospace;margin:48px\">"+
				"<pre style=\"background:linear-gradient(90deg,#F25623,#FFD24A);-webkit-background-clip:text;color:transparent;font-weight:700\""+
				">█▀▄ █ █ █▀█ █▄▀ █ █\n█▀▄ ▀█▀ █ █ █▀▄ █ █\n▀ ▀ ░█░ ▀▀▀ ▀ ▀ ▀▀▀</pre>"+
				"<p>the store is being stocked. meanwhile: <code style=\"color:#9ece6a\">ssh %s</code></p>"+
				"</body>", template.HTMLEscapeString(cfg.Ryolink.Domain), template.HTMLEscapeString(cfg.Ryolink.Domain))
		})
	}
	webAddr := net.JoinHostPort(cfg.Server.WebBind, strconv.Itoa(cfg.Server.WebPort))
	go func() {
		srv := &http.Server{Addr: webAddr, Handler: webMux, ReadHeaderTimeout: 10 * time.Second}
		log.Printf("web: %s (store=%v radio=on)", webAddr, shopInst != nil)
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("web: %v", err)
		}
	}()

	pollStore := poll.NewStore()

	// bartender (persona ships embedded; bartender/soul.md overrides)
	var bt *bartender.Bartender
	if apiKey := cfg.Secret("OPENAI_API_KEY", cfg.API.OpenAIKey); apiKey != "" {
		soul := bartender.LoadSoul(dataDir)
		if tmpl, tmplErr := template.New("soul").Parse(soul); tmplErr == nil {
			var buf bytes.Buffer
			data := map[string]string{
				"RyolinkName": cfg.Ryolink.Name,
				"OwnerName":   cfg.Owner.Name,
				"Domain":      cfg.Ryolink.Domain,
			}
			if err := tmpl.Execute(&buf, data); err == nil {
				soul = buf.String()
			}
		}
		bt = bartender.New(apiKey, soul, st)
		log.Println("bartender: enabled")
	} else {
		log.Println("bartender: disabled (no OPENAI_API_KEY)")
	}

	port := cfg.Server.Port
	if env := os.Getenv("RYOLINK_PORT"); env != "" {
		if n, err := strconv.Atoi(env); err == nil {
			port = n
		}
	}

	// gif search
	var gifClient *gif.KlipyClient
	if klipyKey := cfg.Secret("KLIPY_API_KEY", cfg.API.KlipyKey); klipyKey != "" {
		gifClient = gif.NewKlipyClient(klipyKey)
		log.Println("gif search: enabled")
	} else {
		log.Println("gif search: disabled (no KLIPY_API_KEY)")
	}

	// Reddit feed client (always created — subreddits can be added via --feed-add)
	// OAuth credentials avoid 403 blocks on cloud server IPs
	redditClient := reddit.NewClient(
		cfg.Secret("REDDIT_CLIENT_ID", cfg.API.RedditClientID),
		cfg.Secret("REDDIT_CLIENT_SECRET", cfg.API.RedditSecret),
	)
	feedSubs := st.FeedSubreddits()
	if len(feedSubs) > 0 {
		log.Printf("reddit feed: enabled (%d subreddits)", len(feedSubs))
		go redditClient.FetchMerged(feedSubs, 25)
	} else {
		log.Println("reddit feed: no subreddits configured (use --feed-add)")
	}

	// Mystery engine: embedded case always loads; a mysteries/case01 dir in
	// the data dir overrides it.
	var mysteryEngine *mystery.Engine
	if me, loadErr := mystery.New(filepath.Join(dataDir, "mysteries", "case01")); loadErr != nil {
		log.Printf("mystery: %v", loadErr)
	} else {
		mysteryEngine = me
	}

	// web search
	exaKey := cfg.Secret("EXA_API_KEY", cfg.API.ExaKey)
	searcher := search.New(exaKey)
	if exaKey != "" {
		log.Println("web search: enabled (Exa + DuckDuckGo fallback)")
	} else {
		log.Println("web search: DuckDuckGo only (no EXA_API_KEY)")
	}

	srv, err := server.New(server.Config{
		Host:             cfg.Server.Host,
		Port:             port,
		HostKeyPath:      filepath.Join(hostKeyDir, "id_ed25519"),
		Store:            st,
		Hub:              h,
		JukeboxEngine:    jukeboxEngine,
		SudokuGame:       sudokuGame,
		PollStore:        pollStore,
		Bartender:        bt,
		RyolinkName:      cfg.Ryolink.Name,
		RyolinkDomain:    cfg.Ryolink.Domain,
		Tagline:          cfg.Ryolink.Tagline,
		OwnerName:        cfg.Owner.Name,
		OwnerFingerprint: cfg.Owner.Fingerprint,
		FirstRoom:        cfg.FirstRoom(),
		BarRoom:          cfg.BarRoom(),
		RoomOrder:        cfg.RoomNames(),
		MouseDefault:     cfg.Bind.Mouse != "off",
		RoomTypes:        cfg.RoomTypeMap(),
		Shop:             shopInst,
		GifClient:        gifClient,
		WargameStore:     wargame.New(st.DB()),
		Searcher:         searcher,
		DMStore:          initDMStore(st),
		RedditClient:     redditClient,
		MysteryEngine:    mysteryEngine,
		Guard:            g,
		IdleTimeout:      idleTimeout,
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startPurgeScheduler(st, h, pollStore, mysteryEngine, *cfg)
	if bt != nil {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				bt.DecayMood()
			}
		}()
	}
	go watchBannerFile(st, h)
	go watchAddRoomFile(st, h)
	go watchRenameRoomFile(st, h)
	go watchRemoveRoomFile(st, h)
	go watchBanFile(h)
	go watchPurgeFile(h)
	go watchBartenderToggle(bt)
	go watchDenyFile(g)
	go watchConfigReload(configPath, st)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(ctx); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	log.Printf("%s is open. ssh %s -p %d", cfg.Ryolink.Domain, cfg.Ryolink.Domain, port)

	<-done
	log.Println("ryolink closing...")
	cancel()
	h.BroadcastAll(session.Msg{
		Type: session.MsgSystem,
		Text: "ryolink is closing...",
	})
	srv.Shutdown(5 * time.Second)
	log.Println("goodbye.")
}

// watchDenyFile picks up live --deny / --undeny signals and re-syncs the
// guard's in-memory ban map from the persistent store.
func watchDenyFile(g *guard.Guard) {
	for {
		time.Sleep(1 * time.Second)
		data, err := os.ReadFile(dataPath(denyFile))
		if err != nil {
			continue
		}
		os.Remove(dataPath(denyFile))
		_ = strings.TrimSpace(string(data)) // which cidr changed; a reload is cheap
		g.Reload()
		log.Printf("security: network bans reloaded")
	}
}

func runUpdate() error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("run ryolink --update as ryolink user, not root")
	}

	repoDir, err := executableRepoDir()
	if err != nil {
		return err
	}

	env := updateEnv()
	if err := ensureCleanTrackedFiles(repoDir, env); err != nil {
		return err
	}

	fmt.Println("Fetching latest main...")
	if err := runCommand(repoDir, env, "git", "fetch", "origin", "main"); err != nil {
		return err
	}

	fmt.Println("Pulling latest main...")
	if err := runCommand(repoDir, env, "git", "pull", "--ff-only", "origin", "main"); err != nil {
		return err
	}

	fmt.Println("Building ryolink...")
	if err := runCommand(repoDir, env, "go", "build", "-o", "ryolink.new", "./cmd/ryolink"); err != nil {
		return err
	}
	// Swap atomically; the running service keeps its old inode until restart.
	if err := os.Rename(filepath.Join(repoDir, "ryolink.new"), filepath.Join(repoDir, "ryolink")); err != nil {
		return fmt.Errorf("swap binary: %w", err)
	}

	fmt.Println("Restarting service...")
	if err := runCommand(repoDir, env, "systemctl", "restart", "ryolink"); err != nil {
		if err2 := runCommand(repoDir, env, "sudo", "systemctl", "restart", "ryolink"); err2 != nil {
			fmt.Println("Could not restart automatically — run: sudo systemctl restart ryolink")
		}
	}

	rev, err := commandOutput(repoDir, env, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}

	fmt.Printf("Update complete. Running commit %s\n", rev)
	return nil
}

// dataDir is the resolved directory holding every mutable file: db, host
// keys, logs, and the admin signal files. Set by loadConfig (or defaulted
// for admin verbs that run without a full load).
var dataDir string

// dataPath resolves a runtime file name inside the data dir.
func dataPath(name string) string {
	if dataDir == "" {
		return name
	}
	return filepath.Join(dataDir, name)
}

// findConfig locates ryolink.yaml, in order:
// $RYOLINK_CONFIG, ./ryolink.yaml (repo/dev mode), /etc/ryolink/ryolink.yaml
// (installed server — what the service reads), ~/.config/ryolink/ryolink.yaml
// (per-user mode). The /etc entry means admin verbs find the instance
// anywhere on the box, not just from a directory holding the config.
func findConfig() string {
	if p := os.Getenv("RYOLINK_CONFIG"); p != "" {
		return p
	}
	if _, err := os.Stat("ryolink.yaml"); err == nil {
		abs, aerr := filepath.Abs("ryolink.yaml")
		if aerr == nil {
			return abs
		}
		return "ryolink.yaml"
	}
	if _, err := os.Stat("/etc/ryolink/ryolink.yaml"); err == nil {
		return "/etc/ryolink/ryolink.yaml"
	}
	if cfgDir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(cfgDir, "ryolink", "ryolink.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// resolveDataDir sets dataDir from the config (or its fallbacks) and makes
// sure it exists. Admin verbs call this before touching any file.
func resolveDataDir(cfg *config.Config, configPath string) {
	switch {
	case cfg != nil && cfg.Server.DataDir != "":
		dataDir = cfg.Server.DataDir
	case configPath != "":
		dataDir = filepath.Dir(configPath)
	default:
		dataDir = "."
	}
	os.MkdirAll(dataDir, 0700)
}

// loadConfigForAdmin loads ryolink.yaml if present and resolves the data dir.
// Missing/invalid config is not fatal for admin verbs: they fall back to the
// config dir / cwd, matching the pre-rework behaviour.
func loadConfigForAdmin() *config.Config {
	p := findConfig()
	if p == "" {
		resolveDataDir(nil, "")
		return nil
	}
	cfg, err := config.Load(p)
	if err != nil {
		resolveDataDir(nil, p)
		return nil
	}
	resolveDataDir(cfg, p)
	return cfg
}

func resolvedDBPath() string {
	return dataPath("ryolink.db")
}

func executableRepoDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

func ensureCleanTrackedFiles(repoDir string, env []string) error {
	out, err := commandOutput(repoDir, env, "git", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("repo has local tracked changes; commit or revert them before updating")
	}
	return nil
}

func updateEnv() []string {
	pathParts := []string{"/usr/local/go/bin"}
	if current := os.Getenv("PATH"); current != "" {
		pathParts = append(pathParts, current)
	}
	mergedPath := "PATH=" + strings.Join(pathParts, ":")
	base := os.Environ()
	env := make([]string, 0, len(base)+1)
	replaced := false
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") {
			if !replaced {
				env = append(env, mergedPath)
				replaced = true
			}
			continue
		}
		env = append(env, kv)
	}
	if !replaced {
		env = append(env, mergedPath)
	}
	return env
}

func runCommand(dir string, env []string, name string, args ...string) error {
	resolved, err := resolveCommand(name, env)
	if err != nil {
		return err
	}
	// #nosec G204 — fixed argv; repoDir is the executable's own directory
	cmd := exec.Command(resolved, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandOutput(dir string, env []string, name string, args ...string) (string, error) {
	resolved, err := resolveCommand(name, env)
	if err != nil {
		return "", err
	}
	// #nosec G204 — fixed argv; repoDir is the executable's own directory
	cmd := exec.Command(resolved, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func resolveCommand(name string, env []string) (string, error) {
	if strings.Contains(name, "/") {
		return name, nil
	}

	pathEnv := os.Getenv("PATH")
	for i := len(env) - 1; i >= 0; i-- {
		kv := env[i]
		if strings.HasPrefix(kv, "PATH=") {
			pathEnv = strings.TrimPrefix(kv, "PATH=")
			break
		}
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("exec: %q not found in PATH", name)
}

func initDMStore(st *store.Store) *dm.Store {
	ds, err := dm.New(st.DB())
	if err != nil {
		log.Printf("dm store: %v", err)
		return nil
	}
	return ds
}

// startPurgeScheduler runs the data wipe on the configured weekday+time
// (UTC). A disabled schedule simply never fires; the room is then permanent
// and the operator owns whatever that implies.
func startPurgeScheduler(st *store.Store, h *hub.Hub, ps *poll.Store, me *mystery.Engine, cfg config.Config) {
	if cfg.Purge.Disabled {
		log.Println("purge: disabled by config — data is permanent")
		return
	}
	day, hh, mm, err := cfg.PurgeTime()
	if err != nil {
		log.Printf("purge: bad schedule (%v) — falling back to sunday 23:59 UTC", err)
		day, hh, mm = time.Sunday, 23, 59
	}
	for {
		next := nextPurge(time.Now().UTC(), day, hh, mm)
		timer := time.NewTimer(time.Until(next))
		log.Printf("purge: next sweep %s", next.Format(time.RFC3339))
		<-timer.C

		log.Println("purge starting...")
		h.BroadcastAll(session.Msg{
			Type: session.MsgSystem,
			Text: "The Ryolink has been swept clean.",
		})
		st.PurgeAll()
		ps.Clear()
		if me != nil {
			me.Reset()
		}
		log.Println("purge complete")
	}
}

// nextPurge returns the next occurrence of weekday day at hh:mm (UTC) after
// now. If the slot is today but already past, it rolls to next week.
func nextPurge(now time.Time, day time.Weekday, hh, mm int) time.Time {
	days := (int(day) - int(now.Weekday()) + 7) % 7
	next := time.Date(now.Year(), now.Month(), now.Day()+days, hh, mm, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}

// watchConfigReload re-applies the config-owned tables (wargame flags, feed
// subreddits) when the process gets SIGHUP — i.e. `ryolink reload`. The
// listener itself is untouched; only what the YAML owns moves. Purge
// schedule changes still need a restart, which the example config says.
func watchConfigReload(configPath string, st *store.Store) {
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	for range sighup {
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Printf("reload: %v — keeping current tables", err)
			continue
		}
		if len(cfg.Wargame.Games) > 0 {
			if n, err := wargame.New(st.DB()).Sync(cfg.Wargame.Games); err != nil {
				log.Printf("reload: wargame sync: %v", err)
			} else {
				log.Printf("reload: wargame flags applied (%d changes)", n)
			}
		}
		if len(cfg.Feed.Subreddits) > 0 {
			if err := st.SyncFeedSubreddits(cfg.Feed.Subreddits); err != nil {
				log.Printf("reload: feed sync: %v", err)
			} else {
				log.Printf("reload: feed subreddits applied")
			}
		}
	}
}

func watchBannerFile(st *store.Store, h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(bannerFile))
		if err != nil {
			continue
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}

		os.Remove(dataPath(bannerFile))

		log.Printf("Broadcasting banner: %s", text)
		st.SetBanner(text)
		h.BroadcastAll(session.Msg{
			Type: session.MsgBanner,
			Text: text,
		})
	}
}

func watchAddRoomFile(st *store.Store, h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(addRoomFile))
		if err != nil {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(string(data)))
		if name == "" {
			continue
		}

		os.Remove(dataPath(addRoomFile))

		if st.IsRoom(name) {
			log.Printf("Room #%s already exists", name)
			continue
		}

		if err := st.AddRoom(name); err != nil {
			log.Printf("Failed to add room: %v", err)
			continue
		}

		log.Printf("Room added: #%s", name)
		h.BroadcastAll(session.Msg{
			Type: session.MsgRoomAdded,
			Text: name,
		})
	}
}

func watchRenameRoomFile(st *store.Store, h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(renameRoomFile))
		if err != nil {
			continue
		}

		payload := strings.TrimSpace(string(data))
		os.Remove(dataPath(renameRoomFile))

		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			log.Printf("Invalid rename payload: %q", payload)
			continue
		}
		oldName := strings.ToLower(parts[0])
		newName := strings.ToLower(parts[1])

		if !st.IsRoom(oldName) {
			log.Printf("Room #%s does not exist", oldName)
			continue
		}
		if st.IsRoom(newName) {
			log.Printf("Room #%s already exists", newName)
			continue
		}

		if err := st.RenameRoom(oldName, newName); err != nil {
			log.Printf("Failed to rename room: %v", err)
			continue
		}

		log.Printf("Room renamed: #%s → #%s", oldName, newName)
		h.BroadcastAll(session.Msg{
			Type: session.MsgRoomRenamed,
			Text: oldName,
			Room: newName,
		})
	}
}

func watchRemoveRoomFile(st *store.Store, h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(removeRoomFile))
		if err != nil {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(string(data)))
		if name == "" {
			continue
		}
		os.Remove(dataPath(removeRoomFile))

		if !st.IsRoom(name) {
			log.Printf("Room #%s does not exist", name)
			continue
		}

		if err := st.DeleteRoom(name); err != nil {
			log.Printf("Failed to remove room: %v", err)
			continue
		}

		log.Printf("Room removed: #%s", name)
		h.BroadcastAll(session.Msg{
			Type: session.MsgRoomRemoved,
			Text: name,
		})
	}
}

func watchBanFile(h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(banFile))
		if err != nil {
			continue
		}

		fp := strings.TrimSpace(string(data))
		if fp == "" {
			continue
		}
		os.Remove(dataPath(banFile))

		if h.Kick(fp) {
			log.Printf("Kicked banned user: %s", fp[:16]+"...")
		}
	}
}

func watchPurgeFile(h *hub.Hub) {
	for {
		time.Sleep(1 * time.Second)

		if _, err := os.ReadFile(dataPath(purgeFile)); err != nil {
			continue
		}

		os.Remove(dataPath(purgeFile))

		log.Println("Manual purge signal received, broadcasting to clients")
		h.BroadcastAll(session.Msg{
			Type: session.MsgPurge,
		})
	}
}

func watchBartenderToggle(bt *bartender.Bartender) {
	for {
		time.Sleep(1 * time.Second)

		data, err := os.ReadFile(dataPath(bartenderToggleFile))
		if err != nil {
			continue
		}

		os.Remove(dataPath(bartenderToggleFile))

		if bt == nil {
			log.Println("bartender: toggle ignored (not initialized)")
			continue
		}

		action := strings.TrimSpace(string(data))
		switch action {
		case "off":
			bt.Disable()
			log.Println("bartender: disabled by admin")
		case "on":
			bt.Enable()
			log.Println("bartender: enabled by admin")
		}
	}
}
