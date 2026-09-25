# The ryolink UI guide

How to make the product look like the product: ASCII titles, ASCII icons,
color, and TUI surfaces. Every rule here is either extracted from working
code in `ui/` or paid for by a real bug. Read
[DEVELOPMENT.md](../DEVELOPMENT.md) "UI: Bubble Tea over SSH" first —
that covers the hazards; this covers the craft.

## 1. The visual identity in one box

| Token | Value | Where it's used |
|---|---|---|
| sumi ink bg | `#16161e` | terminal background assumption everywhere |
| sakura text | `#c0caf5` | names, titles, body (`ColorSand`) |
| torii vermilion | `#F25623` | brand stop A, section headers, selected borders (`ColorAmber`) |
| gold | `#FFD24A` | brand stop B, key hints, splash word (`ColorHighlight`) |
| ai blue | `#7aa2f7` | command-ish text, ASCII art, URLs (`ColorCommand`) |
| matcha | `#9ece6a` | confirmations, the `$ curl` line (`ColorGreen`) |
| dim / dimmer | `#7079b3` / `#3b4261` | secondary / chrome text — the hierarchy is a **fade ladder**, not size |

The palette is Tokyo Night-derived and shared with the Ryoku installers
(`ui/styles.go` carries the comment). The brand gradient is literally
vermilion→gold (`BrandGradA/B`). When you need a new color, take it from
Tokyo Night's table, not off the web — the family is: blue-violet hues,
desaturated, all readable on `#16161e`.

**The fade ladder is the layout.** ryolink separates content by color
weight — bright sakura = the thing you're looking at, dim = supporting,
dimmer = chrome (keys, positions, counts). Bold and italic exist but are
sparingly used. Almost nothing uses a background color except selection
rows; if you're reaching for `Background()`, you probably want the ladder
instead.

## 2. ASCII titles (wordmarks)

A store item's `logo:` is either raw multi-line ASCII art or, when it is a
single short line, a **wordmark**: five rows of block letters generated
from text (`ui/wordmark.go`).

```
██  ███   ███ █  █      ← "ARCH" as a wordmark
█  █ █  █ █    █  █
████ ███  █    ████
█  █ █ █  █    █  █
█  █ █  █  ███ █  █
```

The font contract, verbatim from `wordFont`:

- **Exactly 5 rows per glyph.** The renderer joins glyphs with one space
  and assumes row count 5; a 4- or 6-row glyph silently tears the word.
- Glyphs vary in width (3–5 cols) — that's what makes it look like a
  wordmark and not a grid. `I` is 3, `M`/`W`/`X` are 5.
- The character set is `█` (U+2588 full block) and space. Nothing else.
  Half blocks in a wordmark read as a different font.
- `IsWordmark` gates it: single line, ≤10 chars, every rune in the font
  (case-insensitively). Anything else is treated as raw art. So `"ARCH"`,
  `"RYOKU"`, `"CACHY"` become wordmarks; a logo with `!` or a newline
  stays art.

**Adding a glyph:** 5 strings in `wordFont`, `█` for ink. Draw on paper
first; a 4-wide cell gives you strokes of 2 blocks. Keep stems 1-wide and
counters (holes in `A`, `P`, `R`) open on the correct side — a closed
counter turns the letter into `O` at cell size. There's a test culture
here: card heights are derived from the band (`cardHFor` ↔ `logoBand`
share one function "so the geometry and the pixels can never disagree") —
if you change row count, that pairing is where drift would show.

**The 5-row cap is load-bearing in geometry.** The bento grid clips art to
`bentoMaxLogo = 3` rows but gives a wordmark its full 5 and grows the card
(`wordmark_test.go` asserts it). When you place art in any fixed-height
slot, resolve it through one function that both the height math and the
renderer call.

**Raw art in YAML:** multi-line logos go in block scalars. Art with
significant leading spaces (the Arch `▀▄` shapes) needs the explicit
indent indicator — `logo: |2` — or YAML eats the shapes and every card
renders as a left-aligned blob.

## 3. ASCII icons

Small glyphs that mark a kind or a state, sitting inline in text. Two
registers, both from the Unicode blocks geometry:

**Full-block (`█ U+2588`)** for structure: wordmarks, the tankard, big
art. One cell, unambiguous width, renders on every terminal.

**Half/quarter blocks (`▀ ▄ ▌ ▐ ░ ▒ ▓`)** for *texture*: shading,
gradients, the Arch A. Half blocks are a 2×1 pixel grid inside the cell —
top half, bottom half. Compose them the way you'd draw a bitmap: two rows
of `▀▄` give you four sub-rows of resolution, which is how the splash
mountains work.

The existing icon set (all single-cell, all in `ui/`):

```
◆ disc / iso          ⚡ rescue script      □ generic file
● online               ◉ typing             ▸▾ cursor, focus
◂ ▸                    prev / next           ✓  confirmation
↑ ↓ ← →               motion                ⏎  enter (never ↵)
╱                      decorative rule fill   ─ │ ┌ ┐ └ ┘  frames
```

Rules that keep icons from breaking layouts:

- **One cell, or account for it.** Most of these are narrow, but the
  library counts wide runes inconsistently over SSH; anything you measure
  (`lipgloss.Width`) must match what the terminal paints. If a glyph
  shifts a rule by one column on the live box, you used a wide emoji
  where a `▸` belonged. The 力/CJK trap is documented in
  DEVELOPMENT.md — same failure, cheaper fix: pick a box-drawing rune.
- **State changes by color, not by glyph.** Online is `●` dimmer → `●`
  matcha; never `●`→`○`, the shape swap reads as a different object.
  (Verified live during the store rework: the online list pulses by
  color.)
- **Icons label, text explains.** `kindGlyph` marks the card; `kindLabel`
  ("IMAGE"/"SCRIPT") spells it. A bare glyph as the only signal is a
  legend someone has to memorize.

## 4. Color work

**Gradients.** Two tools, one contract (blend in a perceptual space,
never RGB lerp — RGB mud is visible at `#F25623→#FFD24A` midpoints):

- `GradientText(s, c1, c2, bold)` — per-rune HCL blend
  (`ui/gradient.go`), used for the splash word.
- `GradientBar(w, frame)` — a `─` rule sweeping brand vermilion→gold,
  ping-ponged by frame so the seam never snaps (`ui/splash.go`). Store
  cards open with one; it is the brand signature, use it at the top of
  any full-width surface.

For custom stops, `BrandColor(t)` lerps the brand; anything else goes
through `colorful` + `BlendHcl` like `blendColors` does.

**What gets which color** (from the current surfaces — follow the
existing page when adding one):

| Element | Color |
|---|---|
| selected card border | `ColorAmber` (unselected: `ColorBorder`/`ColorDimmer`) |
| name/title | `ColorSand` bold |
| art, URLs | `ColorCommand` blue |
| the fetch line (`$ curl …`) | `ColorGreen` |
| key legend | keys `ColorDimmer`, labels `ColorDim` |
| confirmations | `ColorGreen` with `✓ ` |
| strikes/warnings in place | `ColorDimmer` + `Strikethrough` (missing files) |

Nick colors are a fixed 12-tone family (`NickColors`); assignments are by
index hash — never invent per-user colors ad hoc.

**Dark only.** The palette assumes sumi ink. Don't test on a light
terminal and "fix" it by adding background fills; the product ships
dark, the SSH banner says so.

## 5. Building a TUI surface

### Geometry: measure once, render and hit-test from it

Every clickable/scrollable surface follows the storefront pattern:

1. Parent calls `SetSize(w, h)` and `SetOrigin(topRow)` — the surface
   never assumes where it sits.
2. `View()` computes layout, *records the row math it drew with*
   (`s.top`, `lastStoreModal`), and only then renders.
3. Click/wheel handlers (`itemAtClick`, `inside(x,y)`) replay the same
   numbers. Any decoration row that shifts content (the `/` search bar
   adds a row) must be counted in **both** places, or clicks land on the
   wrong card — this exact drift is why the origin offset moved into one
   variable.
4. Modals hit-test against the placement `Overlay` will give them:
   Overlay centres by `(w-boxW)/2`; cache that in `View()` so `Update`
   (which runs before the draw) can decide "was this click inside the
   card, on the nav row?".

Box math you must not get wrong (learned the hard way):

- lipgloss `Width(n)` means **outer box width n**: content width is
  `n − borders(2) − padding`. A box with `Padding(1,2)` gives prose
  `n−6` columns. Budgeting `n−4` wraps text early and spills footer rows.
- A line longer than the box width *wraps silently* and tears the frame
  (the `▸` of the nav strip landing on its own row). When you pad a line
  to `bodyW` and then indent it by 2, you built a `bodyW+2` line. Every
  full-width row: budget the indent *before* filling.
- `lipgloss.Style` has **no nil**. Use the zero style (`lipgloss.NewStyle()`)
  as the "no styling" marker in tagged line structs.
- A `Width()` style on already-wrapped text **re-wraps per word** —
  width is not a right-pad tool. Pad with `Height()`/manual spaces.
- Right-aligning inside a line = `leftPad(s, w)` (spaces *before* it).
  `s + spaces` is left-align and collides with whatever follows
  (`Ryoku LinuxIMAGE` was exactly this).

### Keys: one verb, one key family

The rules the store rework settled on, now repo convention:

- **Browse vs read never share a key.** Arrows walk the catalogue;
  pgup/pgdown/space page text. Mixing them means the legend can't say
  both, and users can't tell which surface has focus.
- Bare letters (`hjkl`) only on single-purpose surfaces (the modal, the
  grid) where there is no text input to eat them. Any surface with an
  input owns typing; mode changes (search on/off) are explicit (`/`),
  and the mode's own keys come back out via esc.
- Keys that *exist as their own code* (space, enter, esc, arrows) must be
  matched by their bubbletea name (`"space"`, not `" "`). Construct them
  correctly in tests: `tea.KeyPressMsg{Code: tea.KeySpace}` —
  `keyMsg(" ")` is a rune-typed key and will not match.
- A hint printed on screen is a promise: "esc clears" must clear, tested
  or not.

### Status vs chrome: chrome never moves

The footer legend (`⏎ copy · u curl · …`) is fixed. Action feedback
(`✓ checksum copied`, `no checksum on disk yet`) renders on its own
reserved line below it — and the line exists (blank) when there's nothing
to say, so the card is the same height on every state. Same reason the
body window is padded with `Height()`: transient content must not shift
fixed content.

### View purity

`View()` reads only model state — no DB, no clock, no mutation of what
it draws (the grid's `visible()` filter is recomputed from fields; the
cursor clamp is `Update`/`SetItems`'s job). Over SSH with alt-screen full
redraws there is no diffing to save you: an impure `View()` is a stutter,
and `Storefront.SetItems` mutating the caller's slice under a
`sync.OnceValues` catalogue is a corruption bug of the "cached collection
mutation" class — copy before you reorder.

### Proving a surface

1. **Unit: call `View(width, height)` in a test.** Alignment asserts
   (`splash_test.go`), card-height math (`wordmark_test.go`), and
   behavior tests that read the rendered, ANSI-stripped text
   (`storedetail_test.go` walks scroll windows looking for words).
2. **Live: a real PTY, a real terminal.** `make run`, or drive it
   headlessly with python `pyte` + `pty.fork` (this is how the store
   frames were caught: the `▸` wrap, the 2-col overflow, the truncated
   description). Screenshots of TUIs lie; a terminal-emulator screen
   buffer dump does not.
3. When a frame *must* be pixel-true (cursor rendering, colors over
   SSH), remember screenshots can't capture hardware cursors and some
   OSC sequences (clipboard) have no visible effect on the guest —
   assert behavior from the other side (`ryolink status`, server logs).

### The small style sheet (things that read as ryolink)

- Rounded borders, single-char `─`/`│`, never `#` or `=` rules.
- Section header style: `VERMILION BOLD CAPS`, nothing else decorated.
- Keys in legends: UPPERCASE dimmer + lowercase dim description;
  separators are ` · ` — never brackets, pipes, or slashes around keys.
- Numbers of position are centered and dimmer: `5 / 10` reads as chrome.
- Empty states have a voice: *"the shelves are empty — the owner has not
  stocked ryolink.yaml yet."* Never "No items."
- Copy actions echo the *how*: "paste in a browser, or: curl -LO …" —
  the app assumes the user's clipboard may not work over SSH and says
  what to do next.
