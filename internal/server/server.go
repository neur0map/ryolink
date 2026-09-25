package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	bm "charm.land/wish/v2/bubbletea"
	lm "charm.land/wish/v2/elapsed"
	gossh "golang.org/x/crypto/ssh"
	"ryolink/internal/bartender"
	"ryolink/internal/dm"
	"ryolink/internal/gif"
	"ryolink/internal/guard"
	"ryolink/internal/hub"
	"ryolink/internal/identity"
	"ryolink/internal/jukebox"
	"ryolink/internal/mystery"
	"ryolink/internal/poll"
	"ryolink/internal/reddit"
	"ryolink/internal/search"
	"ryolink/internal/session"
	"ryolink/internal/shop"
	"ryolink/internal/store"
	"ryolink/internal/sudoku"
	"ryolink/internal/wargame"
	"ryolink/ui"
)

const mikaName = "Mika" // display name of the bar-keeper

type Config struct {
	Host             string
	Port             int
	HostKeyPath      string
	Store            *store.Store
	Hub              *hub.Hub
	JukeboxEngine    *jukebox.Engine
	SudokuGame       *sudoku.Game
	PollStore        *poll.Store
	Bartender        *bartender.Bartender
	RyolinkName      string
	RyolinkDomain    string
	Tagline          string
	OwnerName        string
	OwnerFingerprint string
	FirstRoom        string
	BarRoom          string
	RoomOrder        []string
	MouseDefault     bool
	RoomTypes        map[string]string
	GifClient        *gif.KlipyClient
	WargameStore     *wargame.Store
	Searcher         *search.Searcher
	DMStore          *dm.Store
	RedditClient     *reddit.Client
	MysteryEngine    *mystery.Engine
	Shop             *shop.Shop    // nil = store room type unavailable
	Guard            *guard.Guard  // nil disables the abuse firewall
	IdleTimeout      time.Duration // 0 = never drop idle sessions
}

type Server struct {
	cfg  Config
	wish *ssh.Server
}

// remoteAddr extracts the peer address string from an ssh context, or "".
func remoteAddr(ctx ssh.Context) string {
	if a := ctx.RemoteAddr(); a != nil {
		return a.String()
	}
	return ""
}

func New(cfg Config) (*Server, error) {
	s := &Server{cfg: cfg}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	// fail records a rejected auth attempt against the peer address; the
	// guard escalates repeated failures to a temporary network ban.
	fail := func(ctx ssh.Context) {
		if cfg.Guard != nil {
			cfg.Guard.RecordAuthFailure(remoteAddr(ctx))
		}
	}

	ops := []ssh.Option{
		wish.WithAddress(addr),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		// Fingerprint nothing about the Go SSH stack: the banner is ours.
		wish.WithVersion("ryolink"), // the lib prepends "SSH-2.0-"
		// ryolink accepts every well-formed public key: the key IS the
		// identity (there are no accounts to authorize). A keyless or
		// malformed attempt is scanner noise — count it against the address.
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			if key == nil {
				fail(ctx)
				return false
			}
			if cfg.Guard != nil {
				cfg.Guard.RecordHandshake(remoteAddr(ctx))
			}
			return true
		}),
		// Password and keyboard-interactive auth do not exist here — but an
		// attempt still counts as a failure, so brute-force scripts burn
		// through their ban budget fast.
		ssh.PasswordAuth(func(ctx ssh.Context, _ string) bool {
			fail(ctx)
			return false
		}),
		ssh.KeyboardInteractiveAuth(func(ctx ssh.Context, _ gossh.KeyboardInteractiveChallenge) bool {
			fail(ctx)
			return false
		}),
		wish.WithMiddleware(
			bm.Middleware(s.teaHandler),
			lm.Middleware(),
		),
	}
	if cfg.IdleTimeout > 0 {
		ops = append(ops, wish.WithIdleTimeout(cfg.IdleTimeout))
	}

	ws, err := wish.NewServer(ops...)
	if err != nil {
		return nil, fmt.Errorf("wish server: %w", err)
	}

	// ---- protocol lockdown ------------------------------------------------
	// ryolink speaks exactly one thing: an interactive terminal session.
	// Everything SSH could otherwise be used for — remote command execution,
	// sftp/scp file transfer, port forwarding (turning the chat box into an
	// attack relay or pivot), environment/agent/X11 requests that leak client
	// state — is refused. The library already restricts channels to "session"
	// only (exec/subsystem/direct-tcpip are rejected at the channel layer);
	// this closes the request layer and the auth layer.

	// Slow-loris and handshake-flood defense: a connection that has not
	// completed the transport handshake in 10s is dropped.
	ws.HandshakeTimeout = 10 * time.Second

	// Max 3 authentication attempts per connection (library default 6):
	// credential-cycling scripts die sooner, and every rejection feeds the
	// guard's per-address ban budget.
	ws.ServerConfigCallback = func(_ ssh.Context) *gossh.ServerConfig {
		return &gossh.ServerConfig{MaxAuthTries: 3}
	}

	// Inside an authenticated session: pty + shell + window-change only.
	// env (client leaks, LC_* injection), subsystem (sftp), x11 and agent
	// forwarding are refused outright.
	ws.SessionRequestCallback = func(_ ssh.Session, request string) bool {
		switch request {
		case "pty-req", "shell", "window-change", "signal", "break":
			return true
		}
		return false
	}

	// Port forwarding: deny both directions explicitly — no relay, no pivot.
	ws.LocalPortForwardingCallback = func(_ ssh.Context, _ string, _ uint32) bool { return false }
	ws.ReversePortForwardingCallback = func(_ ssh.Context, _ string, _ uint32) bool { return false }

	s.wish = ws
	return s, nil
}

func (s *Server) teaHandler(sshSess ssh.Session) (tea.Model, []tea.ProgramOption) {
	pubKey := sshSess.PublicKey()
	if pubKey == nil {
		wish.Fatalln(sshSess, "SSH key required to enter ryolink.")
		return nil, nil
	}

	hash := sha256.Sum256(pubKey.Marshal())
	fingerprint := hex.EncodeToString(hash[:])

	banned, err := s.cfg.Store.IsBanned(fingerprint)
	if err != nil {
		log.Printf("ban check error: %v", err)
	}
	if banned {
		wish.Fatalln(sshSess, "You have been banned from ryolink.")
		return nil, nil
	}

	nickname := identity.DefaultNickname(fingerprint)
	existing, _ := s.cfg.Store.GetUser(fingerprint)
	if existing != nil {
		nickname = existing.Nickname
	}

	s.cfg.Store.UpsertUser(fingerprint, nickname)
	s.cfg.Store.RecordVisitor(fingerprint)
	s.cfg.Store.RecordAllTimeVisitor(fingerprint)

	user, _ := s.cfg.Store.GetUser(fingerprint)
	visitCount := 1
	if user != nil {
		visitCount = user.VisitCount
	}

	colorIndex := identity.ColorIndex(fingerprint)
	flair := identity.HasFlair(visitCount)

	firstRoom := s.cfg.FirstRoom
	// The bartender works the first *chat* room, which is not necessarily
	// the landing room — with the store enabled users land in #store.
	barRoom := s.cfg.BarRoom
	if barRoom == "" {
		barRoom = firstRoom
	}

	sess := session.New(fingerprint, nickname, colorIndex, flair, firstRoom)
	s.cfg.Hub.Register(sess)

	go func() {
		<-sshSess.Context().Done()
		s.cfg.Hub.Unregister(sess)
		s.cfg.Hub.Broadcast(barRoom, session.Msg{
			Type: session.MsgUserLeft,
			Text: fmt.Sprintf("%s left the room", sess.Nickname),
			Room: barRoom,
		})
	}()

	s.cfg.Hub.Broadcast(barRoom, session.Msg{
		Type: session.MsgUserJoined,
		Text: fmt.Sprintf("%s joined ryolink", nickname),
		Room: barRoom,
	})

	// Send recent chat history
	history, _ := s.cfg.Store.RecentMessages(firstRoom, 50)

	// Collect GIF URLs from history to re-render (max 10)
	type gifHistoryItem struct {
		index int
		url   string
	}
	var gifItems []gifHistoryItem
	for i, m := range history {
		if m.GifURL != "" && len(gifItems) < 10 {
			gifItems = append(gifItems, gifHistoryItem{index: i, url: m.GifURL})
		}
	}

	// Pre-render GIF frames for history (background-friendly)
	gifFrameCache := make(map[int]struct {
		frames []string
		delays []int
	})
	if s.cfg.GifClient != nil && len(gifItems) > 0 {
		log.Printf("gif history: re-rendering %d GIFs for joining user", len(gifItems))
		for _, gi := range gifItems {
			data, err := s.cfg.GifClient.FetchGIF(gi.url)
			if err != nil {
				log.Printf("gif history: fetch failed: %v", err)
				continue
			}
			decoded, err := gif.Decode(data)
			if err != nil {
				log.Printf("gif history: decode failed: %v", err)
				continue
			}
			frames := gif.RenderFrames(decoded.Frames, 60)
			gifFrameCache[gi.index] = struct {
				frames []string
				delays []int
			}{frames: frames, delays: decoded.Delays}
			log.Printf("gif history: rendered %d frames for %s", len(frames), gi.url)
		}
	}

	for i, m := range history {
		if m.GifURL != "" {
			if cached, ok := gifFrameCache[i]; ok {
				sess.Send <- session.Msg{
					Type:        session.MsgGif,
					Nickname:    m.Nickname,
					Fingerprint: m.Fingerprint,
					ColorIndex:  m.ColorIndex,
					Text:        m.Text,
					Room:        m.Room,
					Timestamp:   m.CreatedAt,
					GifFrames:   cached.frames,
					GifDelays:   cached.delays,
					GifTitle:    m.Text,
					GifURL:      m.GifURL,
				}
				continue
			}
		}
		if m.RedditURL != "" {
			sess.Send <- session.Msg{
				Type:           session.MsgRedditShare,
				Nickname:       m.Nickname,
				Fingerprint:    m.Fingerprint,
				ColorIndex:     m.ColorIndex,
				Room:           m.Room,
				Timestamp:      m.CreatedAt,
				RedditTitle:    m.RedditTitle,
				RedditSub:      m.RedditSub,
				RedditScore:    m.RedditScore,
				RedditComments: m.RedditComments,
				RedditURL:      m.RedditURL,
			}
			continue
		}
		msgType := session.MsgChat
		if m.IsSystem {
			msgType = session.MsgSystem
		}
		sess.Send <- session.Msg{
			Type:        msgType,
			Nickname:    m.Nickname,
			Fingerprint: m.Fingerprint,
			ColorIndex:  m.ColorIndex,
			Text:        m.Text,
			Room:        m.Room,
			Timestamp:   m.CreatedAt,
		}
	}

	ryolinkState := func() bartender.RyolinkState {
		wc, _ := s.cfg.Store.WeeklyVisitorCount()
		sessions := s.cfg.Hub.Sessions(barRoom)
		var names []string
		for _, sess := range sessions {
			names = append(names, sess.Nickname)
		}
		return bartender.RyolinkState{
			OnlineCount:     s.cfg.Hub.OnlineCount(),
			OnlineNames:     names,
			TimeUTC:         time.Now().UTC(),
			WeeklyVisitors:  wc,
			AllTimeVisitors: s.cfg.Store.AllTimeVisitorCount(),
			ActivePolls:     len(s.cfg.PollStore.ActiveRoomPolls(barRoom)),
		}
	}

	gatherContext := func() []bartender.ChatMsg {
		history, _ := s.cfg.Store.RecentMessages(barRoom, 50)
		var ctx []bartender.ChatMsg
		for _, m := range history {
			if !m.IsSystem {
				ctx = append(ctx, bartender.ChatMsg{Nickname: m.Nickname, Text: m.Text})
			}
		}
		return ctx
	}

	broadcastBartender := func(reply string) {
		btMsg := session.Msg{
			Type:       session.MsgChat,
			Nickname:   mikaName,
			ColorIndex: 6,
			Text:       reply,
			Room:       barRoom,
		}
		s.cfg.Store.SaveMessage(barRoom, "", mikaName, 6, reply, false)
		s.cfg.Hub.Broadcast(barRoom, btMsg)
	}

	onSend := func(msg session.Msg) {
		// DM routing: deliver to recipient only, don't broadcast
		if msg.Type == session.MsgDM {
			parts := strings.SplitN(msg.Text, "\x00", 2)
			if len(parts) == 2 {
				toFP := parts[0]
				// Deliver to recipient if online
				s.cfg.Hub.SendTo(toFP, msg)
			}
			return
		}

		switch msg.Type {
		case session.MsgChat:
			s.cfg.Store.SaveMessage(msg.Room, msg.Fingerprint, msg.Nickname, msg.ColorIndex, msg.Text, false)
		case session.MsgGif:
			s.cfg.Store.SaveMessageWithGif(msg.Room, msg.Fingerprint, msg.Nickname, msg.ColorIndex, msg.Text, false, msg.GifURL)
		case session.MsgRedditShare:
			s.cfg.Store.SaveRedditShare(msg.Room, msg.Fingerprint, msg.Nickname, msg.ColorIndex, msg.RedditURL, msg.RedditTitle, msg.RedditSub, msg.RedditScore, msg.RedditComments)
		case session.MsgSystem, session.MsgUserJoined, session.MsgUserLeft:
			s.cfg.Store.SaveMessage(msg.Room, "", "", 0, msg.Text, true)
		}
		// Trim old messages and GIFs in background
		go s.cfg.Store.TrimMessages(msg.Room, 100, 3)
		s.cfg.Hub.Broadcast(msg.Room, msg)

		if s.cfg.Bartender == nil || s.cfg.Bartender.IsDisabled() || msg.Type != session.MsgChat {
			return
		}

		// Direct @bartender trigger
		if bartender.ShouldRespond(msg.Text, msg.Room, barRoom) {
			if s.cfg.Bartender.CanRespond(msg.Fingerprint) {
				go func() {
					// Keep typing indicator alive until API responds
					done := make(chan struct{})
					go func() {
						ticker := time.NewTicker(3 * time.Second)
						defer ticker.Stop()
						s.cfg.Hub.Broadcast(barRoom, session.Msg{
							Type: session.MsgTyping, Nickname: mikaName, Room: barRoom,
						})
						for {
							select {
							case <-done:
								return
							case <-ticker.C:
								s.cfg.Hub.Broadcast(barRoom, session.Msg{
									Type: session.MsgTyping, Nickname: mikaName, Room: barRoom,
								})
							}
						}
					}()
					// web search if needed
					var searchCtx string
					if s.cfg.Searcher != nil && search.NeedsSearch(msg.Text) {
						if results, err := s.cfg.Searcher.Search(msg.Text); err == nil {
							searchCtx = search.FormatContext(results)
							log.Printf("bartender: web search for %q (%d results)", msg.Text, len(results))
						}
					}
					reply, err := s.cfg.Bartender.Respond(gatherContext(), ryolinkState(), msg.Fingerprint, msg.Nickname, msg.Text, s.cfg.Store.IsOwner(msg.Fingerprint), searchCtx)
					close(done)
					if err != nil {
						log.Printf("bartender error: %v", err)
						broadcastBartender("Wipes the glass and says nothing.")
						return
					}
					broadcastBartender(reply)
				}()
			}
			return
		}

		// Unprompted remark — only on first-room messages
		if msg.Room == barRoom && s.cfg.Bartender.ShouldRemark() {
			go func() {
				done := make(chan struct{})
				go func() {
					ticker := time.NewTicker(3 * time.Second)
					defer ticker.Stop()
					s.cfg.Hub.Broadcast(barRoom, session.Msg{
						Type: session.MsgTyping, Nickname: mikaName, Room: barRoom,
					})
					for {
						select {
						case <-done:
							return
						case <-ticker.C:
							s.cfg.Hub.Broadcast(barRoom, session.Msg{
								Type: session.MsgTyping, Nickname: mikaName, Room: barRoom,
							})
						}
					}
				}()
				reply, err := s.cfg.Bartender.Remark(ryolinkState(), gatherContext())
				close(done)
				if err != nil {
					log.Printf("bartender remark error: %v", err)
					return
				}
				broadcastBartender(reply)
			}()
		}

		// Mystery engine — keyword triggers in lounge chat
		if s.cfg.MysteryEngine != nil && s.cfg.MysteryEngine.IsActive() && msg.Type == session.MsgChat && msg.Room == barRoom {
			result := s.cfg.MysteryEngine.Check(msg.Text)
			if result != nil {
				go func() {
					if result.Solved {
						time.Sleep(time.Duration(2000+rand.IntN(1000)) * time.Millisecond)
						for i, line := range result.Confession {
							if i > 0 {
								time.Sleep(time.Duration(1500+rand.IntN(1000)) * time.Millisecond)
							}
							s.cfg.Hub.Broadcast(barRoom, session.Msg{
								Type:       session.MsgChat,
								Nickname:   result.Killer,
								ColorIndex: rand.IntN(14),
								Text:       line,
								Room:       barRoom,
							})
						}
					} else {
						for _, clue := range result.Clues {
							time.Sleep(time.Duration(1000+rand.IntN(2000)) * time.Millisecond)
							s.cfg.Hub.Broadcast(barRoom, session.Msg{
								Type:       session.MsgChat,
								Nickname:   result.Sender,
								ColorIndex: rand.IntN(14),
								Text:       clue.Text,
								Room:       barRoom,
							})
						}
					}
				}()
			}
		}
	}

	var shopItems func() (string, []ui.StoreItemView)
	if s.cfg.Shop != nil {
		sh := s.cfg.Shop
		shopItems = func() (string, []ui.StoreItemView) {
			raw := sh.Items()
			out := make([]ui.StoreItemView, 0, len(raw))
			for _, it := range raw {
				out = append(out, ui.StoreItemView{
					ID: it.ID, Name: it.Name, Kind: it.Kind,
					Description: it.Description, Details: it.Details, Version: it.Version,
					URL: it.URL, Size: it.Size, SHA256: it.SHA256,
					Downloads: it.Downloads, Missing: it.Missing, Logo: it.Logo,
				})
			}
			return sh.Title(), out
		}
	}
	model := ui.NewApp(sess, s.cfg.Store, s.cfg.Hub, onSend, s.cfg.SudokuGame, s.cfg.PollStore,
		s.cfg.RyolinkName, s.cfg.RyolinkDomain, s.cfg.Tagline,
		s.cfg.OwnerName, s.cfg.OwnerFingerprint, s.cfg.FirstRoom,
		s.cfg.RoomTypes, s.cfg.GifClient, s.cfg.WargameStore,
		s.cfg.DMStore, s.cfg.RedditClient, shopItems, s.cfg.MouseDefault, s.cfg.RoomOrder)
	return model, nil
}

func (s *Server) Start(ctx context.Context) error {
	if s.cfg.JukeboxEngine != nil {
		go s.cfg.JukeboxEngine.Run(ctx)
	}

	log.Printf("%s listening on %s", s.cfg.RyolinkDomain, net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))

	ln, err := net.Listen("tcp", net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if s.cfg.Guard != nil {
		ln = s.cfg.Guard.NewListener(ln)
	}
	return s.wish.Serve(ln)
}

func (s *Server) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.wish.Shutdown(ctx)
}
