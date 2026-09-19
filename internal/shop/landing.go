package shop

// landingHTML is the ryolink store front page: rendered server-side from
// the catalog, styled in the Ryoku visual language (sumi ink, torii
// vermilion, gold), and self-contained (no CDN, no build step). The same
// page a visitor sees is what `ssh ryoku.dev` advertises.
const landingHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — Ryoku</title>
<meta name="description" content="Get Ryoku: the ISO, the recovery script, rescue files. Or chat from your terminal: ssh ryoku.dev">
<style>
  :root {
    --ink: #16161e; --ink2: #1a1b26; --line: #2a2b3d;
    --text: #c0caf5; --dim: #7079b3; --dimmer: #3b4261;
    --vermilion: #F25623; --gold: #FFD24A; --blue: #7aa2f7; --matcha: #9ece6a;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--ink); color: var(--text);
    font: 15px/1.6 ui-monospace, "JetBrains Mono", "Space Grotesk", Menlo, monospace;
  }
  a { color: var(--blue); text-decoration: none; }
  a:hover { text-decoration: underline; }
  header {
    padding: 48px 24px 8px; max-width: 860px; margin: 0 auto;
  }
  pre.mark {
    color: transparent; margin: 0 0 8px; line-height: 1.05; font-size: 12px;
    background: linear-gradient(90deg, var(--vermilion), var(--gold));
    -webkit-background-clip: text; background-clip: text;
    display: inline-block;
  }
  pre.mark span { color: var(--text); }
  h1 { font-size: 15px; font-weight: 600; margin: 0 0 4px; }
  .sub { color: var(--dim); font-style: italic; margin: 0 0 24px; }
  main { max-width: 860px; margin: 0 auto; padding: 0 24px 64px; }
  .card {
    background: var(--ink2); border: 1px solid var(--line); border-radius: 10px;
    padding: 16px 18px; margin: 14px 0;
  }
  .card.missing { opacity: .45; }
  .kind {
    color: var(--gold); font-weight: 700; font-size: 11px; letter-spacing: .12em;
  }
  .name { font-weight: 700; }
  .ver { color: var(--dim); }
  .desc { color: var(--dim); margin: 6px 0 10px; }
  .meta { color: var(--dimmer); font-size: 12px; }
  .actions { margin-top: 10px; display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
  .btn {
    border: 1px solid var(--vermilion); color: var(--vermilion);
    padding: 6px 14px; border-radius: 8px; font-weight: 700;
  }
  .btn:hover { background: var(--vermilion); color: var(--ink); text-decoration: none; }
  .copy { cursor: pointer; color: var(--dim); border: 0; background: none; font: inherit; }
  .copy:hover { color: var(--text); }
  .sha { font-size: 11px; color: var(--dimmer); word-break: break-all; }
  .ssh {
    border: 1px dashed var(--line); border-radius: 10px; padding: 16px 18px;
    margin-top: 32px; display: flex; justify-content: space-between; gap: 16px;
    align-items: center; flex-wrap: wrap;
  }
  .ssh code { color: var(--matcha); font-size: 16px; }
  footer { color: var(--dimmer); font-size: 12px; text-align: center; padding: 24px; }
  .toast {
    position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%);
    background: var(--gold); color: var(--ink); padding: 8px 16px; border-radius: 8px;
    font-weight: 700; opacity: 0; transition: opacity .2s; pointer-events: none;
  }
  .toast.show { opacity: 1; }
</style>
</head>
<body>
<header>
  <pre class="mark">█▀▄ █ █ █▀█ █▄▀ █ █
█▀▄ ▀█▀ █ █ █▀▄ █ █
▀ ▀ ░█░ ▀▀▀ ▀ ▀ ▀▀▀</pre>
  <h1>{{.Title}}</h1>
  <p class="sub">Ryoku is an Arch-based desktop for laptops that try. This is where you get it — the image, the panic button, the files that save a box. Updated {{.Updated}}.</p>
</header>
<main>
  {{if .Items}}
  {{range .Items}}
  <section class="card{{if .Missing}} missing{{end}}">
    <div><span class="kind">{{.Kind}}</span>
      &nbsp;<span class="name">{{.Name}}</span>{{if .Version}} <span class="ver">{{.Version}}</span>{{end}}</div>
    <div class="desc">{{.Description}}</div>
    <div class="meta">{{bytes .Size}}{{if .Downloads}} · {{.Downloads}} downloads{{end}}{{if .Missing}} · staging soon{{end}}</div>
    {{if .SHA256}}<div class="sha">sha256 {{.SHA256}}</div>{{end}}
    <div class="actions">
      <a class="btn" href="{{.URL}}">download</a>
      <button class="copy" data-url="{{.URL}}">copy link</button>
    </div>
  </section>
  {{end}}
  {{else}}
  <div class="card"><div class="desc">The shelves are being stocked — check back soon.</div></div>
  {{end}}

  <div class="ssh">
    <div>want the humans too? <code>ssh ryoku.dev</code> — no account, your SSH key is your identity. rooms, games, a bar called Mika.</div>
  </div>
</main>
<footer>ryolink · part of ryoku · everything in the chat resets sundays, the store does not</footer>
<div class="toast" id="t">copied</div>
<script>
  document.querySelectorAll('.copy').forEach(function (b) {
    b.addEventListener('click', function () {
      navigator.clipboard.writeText(b.dataset.url).then(function () {
        var t = document.getElementById('t');
        t.classList.add('show');
        setTimeout(function () { t.classList.remove('show'); }, 900);
      });
    });
  });
</script>
</body>
</html>`
