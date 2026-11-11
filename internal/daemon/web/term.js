const name = new URLSearchParams(location.search).get('name') || '';
document.getElementById('name').textContent = name;
document.title = `${name} · keepty`;
const state = document.getElementById('state');
const enc = new TextEncoder();

const term = new Terminal({
  cursorBlink: true, fontSize: 13, scrollback: 5000, allowProposedApi: true,
  fontFamily: '"SF Mono", Menlo, "DejaVu Sans Mono", monospace',
  theme: { background: '#000000' },
});
const fit = new FitAddon.FitAddon();
term.loadAddon(fit);
term.open(document.getElementById('xterm'));
fit.fit();

let ws, ctrl = false, backoff = 500;
function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(withToken(`${proto}://${location.host}/v1/terms/${encodeURIComponent(name)}/ws?cols=${term.cols}&rows=${term.rows}`));
  ws.binaryType = 'arraybuffer';
  ws.onopen = () => { state.textContent = 'live'; state.classList.add('on'); backoff = 500; term.focus(); };
  ws.onmessage = e => {
    if (e.data instanceof ArrayBuffer) { term.write(new Uint8Array(e.data)); return; }
    try { const m = JSON.parse(e.data); if (m.t === 'exit') { term.write(`\r\n\x1b[2m[keepty] terminal exited (${m.code})\x1b[0m\r\n`); state.textContent = 'exited'; state.classList.remove('on'); ws.onclose = null; } } catch {}
  };
  ws.onclose = () => {
    state.textContent = 'reconnecting'; state.classList.remove('on');
    setTimeout(connect, backoff); backoff = Math.min(backoff * 2, 8000);
  };
}
function send(bytes) { if (ws && ws.readyState === 1) ws.send(bytes); }
term.onData(d => {
  if (ctrl && d.length === 1) { const c = d.toUpperCase().charCodeAt(0); if (c >= 64 && c <= 95) { d = String.fromCharCode(c - 64); } setCtrl(false); }
  send(enc.encode(d));
});
term.onBinary(d => send(Uint8Array.from(d, c => c.charCodeAt(0))));
function resize() { fit.fit(); if (ws && ws.readyState === 1) ws.send(JSON.stringify({ t: 'resize', cols: term.cols, rows: term.rows })); }
window.addEventListener('resize', resize);
if (window.visualViewport) window.visualViewport.addEventListener('resize', resize);

const ctrlBtn = document.getElementById('ctrl');
function setCtrl(on) { ctrl = on; ctrlBtn.classList.toggle('on', on); }
ctrlBtn.addEventListener('click', () => { setCtrl(!ctrl); term.focus(); });
document.querySelectorAll('#keys button[data-seq]').forEach(b => b.addEventListener('click', () => {
  send(enc.encode(JSON.parse(`"${b.dataset.seq}"`))); term.focus();
}));
connect();
