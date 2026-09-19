package version

// Entry holds a single version's changelog.
type Entry struct {
	Version string
	Changes []string
}

// Changelog lists versions newest-first.
var Changelog = []Entry{
	{
		Version: "0.8",
		Changes: []string{
			"command palette — ctrl+p lists every action, fuzzy filter, enter runs it",
			"keybinds reorganized: ctrl+r opens rooms, ctrl+m toggles cursor mode",
			"storefront is a bento grid — ASCII distro logos, curl + browser lines on each card",
			"splash rebuilt: bigger wordmark, rigid alignment",
			"SSH lockdown proven live — exec/sftp/scp/forwarding refused, banner SSH-2.0-ryolink",
			"SECURITY.md + DEVELOPMENT.md — what's protected, what isn't, what will bite you",
		},
	},
	{
		Version: "0.7",
		Changes: []string{
			"the ryoku store front — #store is the landing room, catalog in ryolink.yaml",
			"web surface: landing page, /api/items, resumable downloads with sha256, mirror redirects, download counts",
			"ryoku visual language — installer palette, RYOKU wordmark with the boot shimmer animation",
			"cursor-first input — click rooms/items, ctrl+m toggles, keys keep working",
			"the bartender is Mika now — @mika",
			"charm libraries bumped to the current crush pins",
		},
	},
	{
		Version: "0.6",
		Changes: []string{
			"single self-contained binary — assets embedded, one file to deploy",
			"ryolink init / up / status / service install — YAML-driven setup",
			"built-in abuse firewall — per-IP budgets, auth-fail + scanner bans, persistent deny list",
			"ryolink --deny / --undeny / --deny-list — live network bans",
			"data_dir — one directory holds everything the instance touches",
			"hardened systemd unit generated from your config",
		},
	},
	{
		Version: "0.5",
		Changes: []string{
			"reddit feed — browse posts, read comments in-terminal",
			"share reddit posts to chat, shared posts persist across reconnects",
			"copy post link to clipboard (` key) uses Reddit permalink, not image URL",
			"reddit focus mode — navigate shared posts in chat with ` key",
			"bartender personality overhaul — warm modern pub vibe, no more edgelord",
			"comment view spacing improvements for readability",
			"share confirmation notice in feed and comment views",
		},
	},
	{
		Version: "0.4",
		Changes: []string{
			"/gif search — animated GIFs in chat via Klipy API",
			"half-block rendering with Floyd-Steinberg dithering",
			"GIF animation in chat (3 active, oldest freeze)",
			"GIF history persists across reconnects",
			"wargame CTF rooms — OverTheWire flag submission",
			"leaderboard with signup, levels, triangular points (F7)",
			"wargame mission briefing header per room",
			"rules modal on first wargame visit, Y to sign up",
			"sidebar reorganized: ROOMS and WARGAMES sections",
			"join room modal separates rooms and wargames",
			"world clock in online panel (NYC, CHI, LDN, BER, TYO)",
			"hacker leaderboard mini-view in sidebar",
			"private DMs — TAB to toggle, /dm @name to open",
			"DM inbox with unread counts and conversation list",
			"live DM delivery to online users",
		},
	},
	{
		Version: "0.3",
		Changes: []string{
			"The Shadow — bartender in the lounge, @bartender to talk",
			"bartender remembers regulars across sessions",
			"bartender drops unprompted remarks when the bar is active",
			"mood system — gets grumpier the more you bother him",
			"gallery notes persist all week, not hourly wipes",
			"Other SSH section in the sidebar (/addssh, /rmssh)",
			"version system + changelog modal (C on splash)",
			"help modal updated with all keybinds and commands",
			"URLs render as styled boxes in chat",
			"text selection works outside gallery",
		},
	},
	{
		Version: "0.2",
		Changes: []string{
			"polls — /poll, /vote, /endpoll",
			"tankard clicker in the sidebar (F6)",
			"drink count survives the weekly purge",
			"@mentions with F4 popup + room badges",
			"all-time visitor count in top bar",
			"ban/unban by nickname from admin CLI",
		},
	},
	{
		Version: "0.1",
		Changes: []string{
			"SSH ryolink with 4 rooms",
			"gallery — sticky notes you can drag around",
			"co-op sudoku in #games",
			"animated splash with floating sparks",
			"weekly purge every Sunday 23:59 UTC",
			"server banner, nick colors, flair",
		},
	},
}
