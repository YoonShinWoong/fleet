// keepty web app: sidebar list, up to four split panes of xterm terminals, and notifications.
const $ = s => document.querySelector(s);
const listEl = $('#list'), panesEl = $('#panes'), emptyEl = $('#empty'), toastsEl = $('#toasts'), summaryEl = $('#summary'), connEl = $('#conn');
const ORDER = { needs_input: 0, working: 1, error: 2, idle: 3, shell: 4, unknown: 5, exited: 6, ended: 7 };
const G = st => t('group.' + st) === 'group.' + st ? st : t('group.' + st);
const GROUP = new Proxy({}, { get: (_, st) => G(String(st)) });
const enc = new TextEncoder();
const coarse = matchMedia('(pointer: coarse) and (hover: none)').matches; // touch devices, not Macs with a trackpad
const mobile = () => matchMedia('(max-width: 800px)').matches;
const settings = Object.assign({ desktop: false, sound: false }, loadJSON('keepty.settings', {}));
let snap = { terms: [], sessions: [], workspaces: [] };
let agents = [], repos = [];
const wsAddOpen = new Set();
const collapsed = new Set(loadJSON('keepty.collapsed', []));
let shortcutRows = [];
let hits = [], hitsQ = '', hitTimer;
const SEARCH_DECOR = { matchBackground: '#f59e0b55', activeMatchBackground: '#f59e0b', matchOverviewRuler: '#f59e0b', activeMatchColorOverviewRuler: '#f59e0b' };
let rows = [];
let prevStatus = null;
let restored = false;
let mobileShow = false;
let ctrl = false;
const seen = {};
const panes = [];
let focusedId = null, paneSeq = 0;

function shortPath(p) {
  p = p.replace(/^\/Users\/[^/]+|^\/home\/[^/]+/, '~');
  if (p.length <= 28) return p;
  const parts = p.split('/').filter(Boolean);
  return '…/' + parts.slice(-2).join('/');
}
function loadJSON(k, d) { try { return JSON.parse(localStorage.getItem(k)) ?? d; } catch { return d; } }
function saveJSON(k, v) { try { localStorage.setItem(k, JSON.stringify(v)); } catch {} }

/* ---------- model ---------- */
function computeRows() {
  const byTerm = {};
  for (const s of snap.sessions) if (s.term && (!byTerm[s.term] || s.updatedAt > byTerm[s.term].updatedAt)) byTerm[s.term] = s;
  const out = snap.terms.map(t => {
    const s = byTerm[t.name];
    let status = s ? s.status : 'shell';
    if (!t.alive) status = 'exited'; else if (status === 'ended' || status === 'unknown') status = 'shell';
    const quiet = Math.round((Date.now() - new Date(t.lastOutputAt)) / 1000);
    const cmd = (t.command || '').split(' ')[0].split('/').pop();
    const last = s && t.alive ? (s.status !== 'working' && s.lastMessage ? s.lastMessage : (s.lastTool || '')) : (t.alive ? `${cmd}${quiet > 5 ? ` · quiet ${age(t.lastOutputAt)}` : ''}` : `exit ${t.exitCode}`);
    return { name: t.name, agent: s ? s.agent : (t.agent || ''), status, last, clients: t.clients, sub: s ? s.subagents : 0,
      at: s && t.alive ? s.updatedAt : t.startedAt, alive: t.alive, exitCode: t.exitCode, cwd: t.cwd, repo: t.repo || '', workspace: t.workspace || '', model: (s && s.model) || t.model || '', restored: !!t.restored };
  });
  return out.sort((a, b) => (ORDER[a.status] ?? 9) - (ORDER[b.status] ?? 9) || new Date(b.at) - new Date(a.at));
}
const rowOf = name => rows.find(r => r.name === name);
const statusOf = name => (rowOf(name) || {}).status || 'unknown';
function visibleRows() {
  const q = $('#filter').value.trim().toLowerCase();
  return q ? rows.filter(r => r.name.toLowerCase().includes(q) || (r.last || '').toLowerCase().includes(q)) : rows;
}

/* ---------- sidebar ---------- */
function renderList() {
  rows = computeRows();
  const vis = visibleRows();
  const q = $('#filter').value.trim();
  const groups = new Map();
  for (const r of vis) { const k = r.workspace; if (!groups.has(k)) groups.set(k, []); groups.get(k).push(r); }
  if (!q) for (const w of snap.workspaces) if (!groups.has(w.name)) groups.set(w.name, []);
  const wsByName = Object.fromEntries((snap.workspaces || []).map(w => [w.name, w]));
  const rank = rs => rs.length ? Math.min(...rs.map(r => ORDER[r.status] ?? 9)) : 8;
  const keys = [...groups.keys()].sort((a, b) => {
    const d = rank(groups.get(a)) - rank(groups.get(b)); if (d) return d;
    if (!a) return 1; if (!b) return -1;
    return new Date(wsByName[b]?.createdAt || 0) - new Date(wsByName[a]?.createdAt || 0);
  });
  let html = '', idx = 0;
  shortcutRows = [];
  for (const k of keys) {
    const rs = groups.get(k), w = wsByName[k];
    const roll = rs.length ? rs.reduce((a, r) => (ORDER[r.status] ?? 9) < (ORDER[a] ?? 9) ? r.status : a, rs[0].status) : 'unknown';
    const key = k || '__other';
    const isCol = collapsed.has(key);
    const waitingN = rs.filter(r => r.status === 'needs_input').length;
    const count = isCol ? `<span class="count">${rs.length}${waitingN ? ` · <b>${t('count.input', { n: waitingN })}</b>` : ''}</span>` : '';
    if (k) {
      const meta = w ? (w.kind === 'worktree' ? `${w.repo} · ${w.branch}` : shortPath(w.path || '')) : '';
      html += `<div class="ws s-${roll}${isCol ? ' col' : ''}" data-ws="${esc(k)}"><span class="chev">▾</span><span class="dot"></span><span class="wsname" title="${esc(k)}">${esc(k)}</span><span class="wsmeta" title="${esc(w?.path || '')}">${esc(meta)}</span><span class="spacer"></span>${count}<button data-act="wsadd" title="${t('ws.add')}">+</button>${rs.length ? '' : `<button data-act="wsrm" title="${t('ws.forget')}">×</button>`}</div>`;
      if (isCol) continue;
      if (wsAddOpen.has(k)) html += `<div class="wsadd" data-ws="${esc(k)}">${agents.map(a => `<button data-act="wsopen" data-agent="${esc(a.name)}">${esc(a.name)}</button>`).join('')}</div>`;
      if (!rs.length && !wsAddOpen.has(k)) html += `<div class="ws-empty">${t('ws.empty')}</div>`;
    } else if (rs.length) {
      html += `<div class="ws s-${roll}${isCol ? ' col' : ''}" data-ws="__other"><span class="chev">▾</span><span class="dot"></span><span class="wsname">${t('ws.other')}</span><span class="wsmeta">${t('ws.otherMeta')}</span><span class="spacer"></span>${count}</div>`;
      if (isCol) continue;
    }
    for (const r of rs) {
      const n = idx < 9 ? idx + 1 : ''; idx++;
      shortcutRows.push(r);
      const open = panes.some(p => p.name === r.name);
      const attention = r.status === 'needs_input' && seen[r.name] !== 'needs_input';
      html += `<div class="item in-ws s-${r.status}${open ? ' open' : ''}${attention ? ' attention' : ''}" data-name="${esc(r.name)}">
        <span class="dot"></span>
        <div class="col">
          <div class="l1"><span class="name">${esc(r.name)}</span>${r.agent ? `<span class="agent">${esc(r.agent)}</span>` : ''}${r.model ? `<span class="model">${esc(r.model)}</span>` : ''}<span class="age">${age(r.at)}</span></div>
          <div class="l2">${esc(r.last || GROUP[r.status] || r.status)}${r.sub ? ` · ${r.sub} sub` : ''}</div>
          <div class="l3">${esc(GROUP[r.status] || r.status)}${r.restored ? ' · ' + t('item.restored') : ''}${r.clients ? ` · ${r.clients === 1 ? t('viewers1') : t('viewers', { n: r.clients })}` : ''}${n ? ` · <kbd>⌥${n}</kbd>` : ''}</div>
        </div>
        <div class="acts"><button data-act="split" title="${t('item.split')}">⫼</button><button data-act="kill" title="${t('item.kill')}">×</button></div>
      </div>`;
    }
  }
  if (hitsQ && $('#filter').value.trim() === hitsQ) {
    html += `<div class="group">${t('search.group', { n: hits.length ? (hits.length === 1 ? t('search.terms1') : t('search.terms', { n: hits.length })) : t('search.none') })}</div>`;
    for (const h of hits) {
      html += `<div class="hit" data-name="${esc(h.term)}" data-q="${esc(hitsQ)}"><div class="hn">${esc(h.term)}${h.workspace ? `<span class="agent">${esc(h.workspace)}</span>` : ''}<span class="age">×${h.count}</span></div>${(h.lines || []).map(l => `<div class="hl">${highlight(l, hitsQ)}</div>`).join('')}</div>`;
    }
  }
  listEl.innerHTML = html || `<div class="none">${t('list.none')}</div>`;
  const waiting = rows.filter(r => r.status === 'needs_input').length, working = rows.filter(r => r.status === 'working').length;
  summaryEl.textContent = [waiting ? (waiting === 1 ? t('summary.waiting1') : t('summary.waiting', { n: waiting })) : '', working ? t('summary.working', { n: working }) : ''].filter(Boolean).join(' · ');
  document.title = (waiting ? `(${waiting}) ` : '') + 'keepty';
  for (const p of panes) updatePaneHead(p);
}

listEl.addEventListener('click', async e => {
  const act = e.target.closest('[data-act]')?.dataset.act;
  const wsEl = e.target.closest('[data-ws]');
  const head = e.target.closest('.ws');
  if (head && !act) {
    const key = head.dataset.ws || '__other';
    collapsed.has(key) ? collapsed.delete(key) : collapsed.add(key);
    saveJSON('keepty.collapsed', [...collapsed]);
    renderList();
    return;
  }
  if (wsEl && (act === 'wsadd' || act === 'wsrm' || act === 'wsopen')) {
    const ws = wsEl.dataset.ws;
    if (act === 'wsadd') { wsAddOpen.has(ws) ? wsAddOpen.delete(ws) : wsAddOpen.add(ws); renderList(); return; }
    if (act === 'wsopen') {
      const r = await api('/v1/terms', { method: 'POST', body: { workspace: ws, agent: e.target.dataset.agent } });
      if (!r.ok) return toast(await r.text());
      wsAddOpen.delete(ws);
      openIn(focusedPane(), (await r.json()).name); return;
    }
    if (act === 'wsrm') {
      const b = e.target.closest('button');
      if (!b.classList.contains('arm')) { b.classList.add('arm'); b.textContent = t('ws.forgetConfirm'); setTimeout(() => { b.classList.remove('arm'); b.textContent = '×'; }, 3000); return; }
      const r = await api(`/v1/workspaces/${encodeURIComponent(ws)}`, { method: 'DELETE' });
      if (!r.ok) toast(await r.text());
      return;
    }
  }
  const hit = e.target.closest('.hit');
  if (hit) { openIn(focusedPane(), hit.dataset.name); setTimeout(() => toggleSearch(focusedPane(), hit.dataset.q), 500); return; }
  const item = e.target.closest('.item'); if (!item) return;
  const name = item.dataset.name;
  if (act === 'split') { addPane(name); return; }
  if (act === 'kill') {
    const b = e.target.closest('button');
    if (!b.classList.contains('arm')) { b.classList.add('arm'); b.textContent = t('item.killConfirm'); setTimeout(() => { b.classList.remove('arm'); b.textContent = '×'; }, 3000); return; }
    const r = await api(`/v1/terms/${encodeURIComponent(name)}`, { method: 'DELETE' });
    if (!r.ok) toast(await r.text());
    for (const p of panes.filter(p => p.name === name)) closePane(p);
    return;
  }
  openIn(focusedPane() || addPane(), name);
});
$('#filter').addEventListener('input', () => {
  const q = $('#filter').value.trim();
  clearTimeout(hitTimer);
  if (q.length < 2) { hits = []; hitsQ = ''; renderList(); return; }
  renderList();
  hitTimer = setTimeout(async () => {
    try { const r = await api('/v1/search?q=' + encodeURIComponent(q)); hits = r.ok ? await r.json() : []; } catch { hits = []; }
    hitsQ = q; renderList();
  }, 250);
});
function highlight(line, q) {
  const safe = esc(line), i = safe.toLowerCase().indexOf(esc(q).toLowerCase());
  if (i < 0) return safe;
  return safe.slice(0, i) + '<mark>' + safe.slice(i, i + esc(q).length) + '</mark>' + safe.slice(i + esc(q).length);
}

/* ---------- panes ---------- */
const focusedPane = () => panes.find(p => p.id === focusedId) || null;

function addPane(name) {
  if (panes.length >= 4) { toast(t('toast.max')); return focusedPane(); }
  const id = ++paneSeq;
  const el = document.createElement('div');
  el.className = 'pane'; el.dataset.id = id;
  el.innerHTML = `<div class="phead"><span class="dot"></span><span class="pname"></span><span class="pstatus"></span><span class="spacer"></span><button data-act="search" title="${t('pane.find')}">find</button><button data-act="model" title="${t('pane.model')}">model</button><button data-act="handoff" title="${t('pane.handoff')}">↪</button><button data-act="max" title="${t('pane.max')}">⤢</button><button data-act="close" title="${t('pane.close')}">×</button></div><div class="psearch" hidden><input placeholder="${t('search.ph')}" autocomplete="off" spellcheck="false"><span class="cnt"></span><button data-act="prev" title="${t('search.prev')}">↑</button><button data-act="next" title="${t('search.next')}">↓</button><button data-act="sclose" title="${t('search.close')}">×</button></div><div class="pbody"><div class="choose">${t('pane.choose')}</div></div>`;
  panesEl.appendChild(el);
  const p = { id, name: null, el, body: el.querySelector('.pbody'), term: null, fit: null, ws: null, ro: null, backoff: 500 };
  panes.push(p);
  el.addEventListener('mousedown', () => focusPane(p.id), true);
  el.addEventListener('touchstart', () => focusPane(p.id), { capture: true, passive: true });
  el.querySelector('.phead').addEventListener('click', e => {
    const act = e.target.closest('[data-act]')?.dataset.act;
    if (act === 'close') closePane(p);
    if (act === 'max') p.el.classList.toggle('max');
    if (act === 'model') pickModel(p);
    if (act === 'handoff') openHandoff(p);
    if (act === 'search') toggleSearch(p);
  });
  const sbar = el.querySelector('.psearch'), sin = sbar.querySelector('input');
  sbar.addEventListener('click', e => {
    const a = e.target.closest('[data-act]')?.dataset.act;
    if (a === 'prev') doFind(p, -1);
    if (a === 'next') doFind(p, 1);
    if (a === 'sclose') closeSearch(p);
  });
  sin.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); doFind(p, e.shiftKey ? -1 : 1); }
    else if (e.key === 'Escape') { e.preventDefault(); closeSearch(p); }
  });
  sin.addEventListener('input', () => doFind(p, 1, true));
  focusPane(id);
  if (name) assign(p, name);
  layout(); saveLayout();
  return p;
}

function assign(p, name) {
  teardown(p);
  p.name = name;
  p.term = new Terminal({ cursorBlink: true, fontSize: 13, scrollback: 5000, allowProposedApi: true, fontFamily: '"SF Mono", Menlo, "DejaVu Sans Mono", monospace', theme: { background: '#000000' } });
  p.fit = new FitAddon.FitAddon();
  p.term.loadAddon(p.fit);
  if (window.SearchAddon) {
    p.search = new SearchAddon.SearchAddon();
    p.term.loadAddon(p.search);
    p.search.onDidChangeResults?.(({ resultIndex, resultCount }) => {
      const cnt = p.el.querySelector('.psearch .cnt');
      cnt.textContent = resultCount > 0 ? `${resultIndex + 1}/${resultCount}` : (p.el.querySelector('.psearch input').value ? '0' : '');
    });
  }
  p.term.open(p.body);
  p.term.attachCustomKeyEventHandler(e => { if (e.type === 'keydown' && e.altKey && handleShortcut(e)) { e.keeptyHandled = true; return false; } return true; });
  p.term.onData(d => {
    if (ctrl && d.length === 1) { const c = d.toUpperCase().charCodeAt(0); if (c >= 64 && c <= 95) d = String.fromCharCode(c - 64); setCtrl(false); }
    send(p, enc.encode(d));
  });
  p.term.onBinary(d => send(p, Uint8Array.from(d, c => c.charCodeAt(0))));
  p.ro = new ResizeObserver(() => scheduleFit(p));
  p.ro.observe(p.body);
  requestAnimationFrame(() => { if (!p.term) return; p.fit.fit(); connect(p); p.term.focus(); });
  seen[name] = statusOf(name);
  updatePaneHead(p); renderList(); saveLayout();
}

function teardown(p) {
  if (p.ws) { p.ws.onclose = null; p.ws.close(); p.ws = null; }
  if (p.ro) { p.ro.disconnect(); p.ro = null; }
  if (p.term) { p.term.dispose(); p.term = null; }
  p.search = null;
  const sb = p.el.querySelector('.psearch'); if (sb) { sb.hidden = true; sb.querySelector('input').value = ''; sb.querySelector('.cnt').textContent = ''; }
  p.body.innerHTML = '';
  p.name = null;
}

function closePane(p) {
  teardown(p);
  p.el.remove();
  panes.splice(panes.indexOf(p), 1);
  if (focusedId === p.id) focusPane(panes.length ? panes[panes.length - 1].id : null);
  layout(); renderList(); saveLayout();
}

function focusPane(id) {
  focusedId = id;
  for (const p of panes) p.el.classList.toggle('focused', p.id === id);
  const p = focusedPane();
  if (p && p.name) { const was = seen[p.name]; seen[p.name] = statusOf(p.name); if (was !== seen[p.name]) renderList(); }
}

function openIn(p, name) {
  if (!p) p = addPane();
  if (p.name !== name) assign(p, name);
  if (mobile()) { mobileShow = true; layout(); }
  focusPane(p.id);
  if (p.term) p.term.focus();
}

function updatePaneHead(p) {
  const head = p.el.querySelector('.phead');
  const r = p.name ? rowOf(p.name) : null;
  head.className = 'phead' + (r ? ' s-' + r.status : '');
  head.querySelector('.pname').textContent = p.name || '';
  head.querySelector('.pstatus').textContent = r ? [r.workspace, r.agent ? r.agent + (r.model ? ' / ' + r.model : '') : '', GROUP[r.status] || r.status, r.last].filter(Boolean).join(' · ') : '';
}

function layout() {
  panesEl.className = 'n' + Math.min(panes.length, 4);
  emptyEl.hidden = panes.length > 0;
  const show = mobile() ? mobileShow && panes.length > 0 : panes.length > 0;
  document.body.classList.toggle('show-panes', show);
  $('#back').hidden = !(mobile() && show);
  $('#keys').hidden = !(coarse && show);
  for (const p of panes) scheduleFit(p);
}
window.addEventListener('resize', layout);
$('#back').addEventListener('click', () => { mobileShow = false; layout(); });

function saveLayout() { saveJSON('keepty.layout', { panes: panes.map(p => p.name), focused: panes.findIndex(p => p.id === focusedId) }); }
function restoreLayout() {
  restored = true;
  const saved = loadJSON('keepty.layout', null);
  const names = new Set(snap.terms.map(t => t.name));
  if (saved && Array.isArray(saved.panes)) {
    for (const n of saved.panes) if (n && names.has(n)) addPane(n);
    if (panes[saved.focused]) focusPane(panes[saved.focused].id);
  }
  if (!panes.length && !mobile()) { const r = rows.find(x => x.alive); if (r) addPane(r.name); }
  mobileShow = false; layout();
  if (location.hash === '#waiting') { history.replaceState(null, '', location.pathname); jumpWaiting(); }
}
function jumpWaiting() {
  const cur = focusedPane()?.name;
  const waiting = rows.filter(r => r.status === 'needs_input');
  const next = waiting.find(r => r.name !== cur) || waiting[0];
  if (next) openIn(focusedPane(), next.name); else toast(t('notify.none'), false);
}
window.addEventListener('hashchange', () => { if (location.hash === '#waiting') { history.replaceState(null, '', location.pathname); jumpWaiting(); } });

/* ---------- terminal transport ---------- */
function connect(p) {
  if (!p.name || !p.term) return;
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const ws = new WebSocket(withToken(`${proto}://${location.host}/v1/terms/${encodeURIComponent(p.name)}/ws?cols=${p.term.cols}&rows=${p.term.rows}`));
  ws.binaryType = 'arraybuffer';
  p.ws = ws;
  ws.onopen = () => { p.backoff = 500; p.el.classList.remove('offline'); };
  ws.onmessage = e => {
    if (!p.term) return;
    if (e.data instanceof ArrayBuffer) { p.term.write(new Uint8Array(e.data)); return; }
    try {
      const m = JSON.parse(e.data);
      if (m.t === 'exit') { p.term.write(`\r\n\x1b[2m[keepty] ${t('pane.exited', { code: m.code })}\x1b[0m\r\n`); ws.onclose = null; p.el.classList.add('offline'); }
    } catch {}
  };
  ws.onclose = () => {
    if (p.ws !== ws) return;
    p.el.classList.add('offline');
    setTimeout(() => { if (p.ws === ws && p.name) connect(p); }, p.backoff);
    p.backoff = Math.min(p.backoff * 2, 8000);
  };
}
function send(p, bytes) { if (p && p.ws && p.ws.readyState === 1) p.ws.send(bytes); }
function scheduleFit(p) {
  clearTimeout(p.fitTimer);
  p.fitTimer = setTimeout(() => {
    if (!p.term || !p.body.clientWidth) return;
    p.fit.fit();
    if (p.ws && p.ws.readyState === 1) p.ws.send(JSON.stringify({ t: 'resize', cols: p.term.cols, rows: p.term.rows }));
  }, 60);
}

/* ---------- in-pane search ---------- */
function toggleSearch(p, q) {
  if (!p || !p.term) return;
  const sb = p.el.querySelector('.psearch'), sin = sb.querySelector('input');
  if (!sb.hidden && q === undefined && document.activeElement === sin) { closeSearch(p); return; }
  sb.hidden = false;
  if (typeof q === 'string') sin.value = q;
  sin.focus(); sin.select();
  if (sin.value) doFind(p, 1, true);
}
function doFind(p, dir, incremental = false) {
  if (!p.search) return toast(t('search.addon'));
  const sin = p.el.querySelector('.psearch input'), q = sin.value;
  if (!q) { p.search.clearDecorations(); p.el.querySelector('.psearch .cnt').textContent = ''; return; }
  const opts = { decorations: SEARCH_DECOR, caseSensitive: false, incremental };
  dir < 0 ? p.search.findPrevious(q, opts) : p.search.findNext(q, opts);
}
function closeSearch(p) {
  const sb = p.el.querySelector('.psearch');
  sb.hidden = true;
  p.search?.clearDecorations();
  p.term?.focus();
}

/* ---------- notifications ---------- */
function detectTransitions() {
  const cur = {};
  for (const r of rows) cur[r.name] = r.status;
  if (prevStatus) {
    for (const [name, st] of Object.entries(cur)) {
      const was = prevStatus[name];
      if (was === st) continue;
      const r = rowOf(name);
      if (st === 'needs_input') notify(name, t('notify.needs'), r.last, 'needs');
      else if (was === 'working' && st === 'idle') notify(name, t('notify.done'), r.last, 'idle');
      else if (st === 'exited' && was && was !== 'exited') notify(name, t('notify.exited', { code: r.exitCode }), '', 'info');
      else if (st === 'error') notify(name, t('notify.error'), r.last, 'err');
    }
  }
  prevStatus = cur;
}
function notify(name, title, body, kind) {
  const t = document.createElement('div');
  t.className = 'toast ' + kind;
  t.innerHTML = `<b>${esc(name)}</b> · ${esc(title)}${body ? `<div class="tb">${esc(body)}</div>` : ''}`;
  t.onclick = () => { openIn(focusedPane(), name); t.remove(); };
  toastsEl.appendChild(t);
  setTimeout(() => t.remove(), 8000);
  if (settings.sound) beep(kind === 'needs' ? 880 : 660);
  if (settings.desktop && 'Notification' in window && Notification.permission === 'granted') {
    const n = new Notification(`${name} · ${title}`, { body: body || '', tag: 'keepty-' + name, icon: 'icon.svg' });
    n.onclick = () => { window.focus(); openIn(focusedPane(), name); n.close(); };
  }
}
let audio;
function beep(freq) {
  try {
    audio = audio || new (window.AudioContext || window.webkitAudioContext)();
    const o = audio.createOscillator(), g = audio.createGain();
    o.frequency.value = freq; o.connect(g); g.connect(audio.destination);
    g.gain.setValueAtTime(0.08, audio.currentTime); g.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + 0.18);
    o.start(); o.stop(audio.currentTime + 0.2);
  } catch {}
}
const bell = $('#bell'), snd = $('#sound');
function paintSettings() {
  bell.textContent = settings.desktop ? '🔔' : '🔕'; bell.classList.toggle('on', settings.desktop);
  snd.textContent = settings.sound ? '🔊' : '🔇'; snd.classList.toggle('on', settings.sound);
}
bell.addEventListener('click', async () => {
  if (!settings.desktop) {
    if (!('Notification' in window)) return toast(t('notify.unsupported'));
    if (Notification.permission !== 'granted') { const perm = await Notification.requestPermission(); if (perm !== 'granted') return toast(t('notify.denied')); }
  }
  settings.desktop = !settings.desktop; saveJSON('keepty.settings', settings); paintSettings();
});
snd.addEventListener('click', () => { settings.sound = !settings.sound; saveJSON('keepty.settings', settings); paintSettings(); if (settings.sound) beep(660); });
paintSettings();

/* ---------- shortcuts ---------- */
function handleShortcut(e) {
  if (!e.altKey || e.ctrlKey || e.metaKey) return false;
  const code = e.code;
  if (/^Digit[1-9]$/.test(code)) { const r = shortcutRows[+code.slice(5) - 1]; if (r) openIn(focusedPane(), r.name); e.preventDefault(); return true; }
  if (code === 'KeyN') { jumpWaiting(); e.preventDefault(); return true; }
  if (code === 'KeyS') { addPane(null); e.preventDefault(); return true; }
  if (code === 'KeyW') { const p = focusedPane(); if (p) closePane(p); e.preventDefault(); return true; }
  if (code === 'KeyM') { const p = focusedPane(); if (p) p.el.classList.toggle('max'); e.preventDefault(); return true; }
  if (code === 'KeyF') { toggleSearch(focusedPane()); e.preventDefault(); return true; }
  return false;
}
document.addEventListener('keydown', e => { if (e.keeptyHandled) return; if (handleShortcut(e)) e.keeptyHandled = true; });

/* ---------- mobile key bar ---------- */
const ctrlBtn = $('#ctrl');
function setCtrl(on) { ctrl = on; ctrlBtn.classList.toggle('on', on); }
ctrlBtn.addEventListener('click', () => { setCtrl(!ctrl); focusedPane()?.term?.focus(); });
document.querySelectorAll('#keys button[data-seq]').forEach(b => b.addEventListener('click', () => {
  send(focusedPane(), enc.encode(JSON.parse(`"${b.dataset.seq}"`))); focusedPane()?.term?.focus();
}));

/* ---------- new dialog ---------- */
const dlg = $('#newdlg'), form = $('#newform');
let agentSel = 'claude';
async function loadMeta() {
  try { agents = await (await api('/v1/agents')).json(); repos = await (await api('/v1/repos')).json(); } catch {}
  if (!agents.length) agents = [{ name: 'shell' }, { name: 'claude' }, { name: 'codex' }];
  paintAgentSeg();
}
function openDialog(presetWs) {
  const wssel = $('#wssel');
  wssel.innerHTML = `<option value="__new">${t('dlg.newWs')}</option>` + (snap.workspaces || []).map(w => `<option value="${esc(w.name)}">${esc(w.name)}${w.repo ? ' · ' + esc(w.repo) : ''}</option>`).join('');
  wssel.value = presetWs && (snap.workspaces || []).some(w => w.name === presetWs) ? presetWs : '__new';
  const srcsel = $('#srcsel');
  srcsel.innerHTML = repos.map(r => `<option value="${esc(r.name)}">${esc(t('dlg.repoOpt', { name: r.name }))}</option>`).join('') + `<option value="__path">${t('dlg.pathOpt')}</option>`;
  if (!repos.length) srcsel.value = '__path';
  const names = agents.map(a => a.name);
  if (!names.includes(agentSel)) agentSel = names.includes('claude') ? 'claude' : names[0];
  paintAgentSeg();
  syncDialog();
  dlg.showModal();
  setTimeout(() => (dlg.querySelector('#wsnew[hidden]') ? form.querySelector('[name=name]') : form.querySelector('[name=wsname]')).focus(), 0);
}
function paintAgentSeg() {
  $('#agentseg').innerHTML = agents.map(a => `<button type="button" data-agent="${esc(a.name)}" class="${a.name === agentSel ? 'on' : ''}" title="${esc(a.command || t('dlg.shell'))}">${esc(a.name)}</button>`).join('');
  const a = agents.find(x => x.name === agentSel);
  $('#modelrow').hidden = agentSel === 'shell';
  $('#modellist').innerHTML = (a?.models || []).map(m => `<option value="${esc(m)}">`).join('');
  form.querySelector('[name=model]').placeholder = a?.models?.length ? t('dlg.model.eg', { list: a.models.join(', ') }) : t('dlg.model.ph');
}
function syncDialog() {
  const isNew = $('#wssel').value === '__new';
  $('#wsnew').hidden = !isNew;
  form.querySelector('[name=wsname]').required = isNew;
  const isPath = $('#srcsel').value === '__path';
  $('#pathrow').hidden = !isPath;
  $('#branchrow').hidden = isPath;
}
$('#newbtn').addEventListener('click', () => openDialog());
$('#dlgclose').addEventListener('click', () => dlg.close());
$('#wssel').addEventListener('change', syncDialog);
$('#srcsel').addEventListener('change', syncDialog);
$('#agentseg').addEventListener('click', e => { const b = e.target.closest('[data-agent]'); if (!b) return; agentSel = b.dataset.agent; paintAgentSeg(); });
form.addEventListener('submit', async e => {
  e.preventDefault();
  const f = new FormData(form);
  let ws = f.get('workspace');
  if (ws === '__new') {
    const body = { name: f.get('wsname'), branch: f.get('branch') || '' };
    if (f.get('source') === '__path') body.path = f.get('path'); else body.repo = f.get('source');
    const r = await api('/v1/workspaces', { method: 'POST', body });
    if (!r.ok) return toast(await r.text());
    ws = body.name;
  }
  const r2 = await api('/v1/terms', { method: 'POST', body: { workspace: ws, agent: agentSel, name: f.get('name') || '', model: agentSel === 'shell' ? '' : (f.get('model') || '') } });
  if (!r2.ok) return toast(await r2.text());
  const info = await r2.json();
  form.reset(); dlg.close();
  openIn(focusedPane(), info.name);
});

/* ---------- ask + model + handoff ---------- */
const askdlg = $('#askdlg'), askform = $('#askform');
function ask(title, placeholder, suggestions) {
  return new Promise(resolve => {
    $('#asktitle').textContent = title;
    const inp = $('#askinput');
    inp.value = ''; inp.placeholder = placeholder || '';
    $('#asklist').innerHTML = (suggestions || []).map(v => `<option value="${esc(v)}">`).join('');
    const done = v => { askdlg.close(); cleanup(); resolve(v); };
    const onSubmit = e => { e.preventDefault(); done(inp.value.trim()); };
    const onCancel = () => done(null);
    const cleanup = () => { askform.removeEventListener('submit', onSubmit); $('#askcancel').removeEventListener('click', onCancel); askdlg.removeEventListener('cancel', onCancel); };
    askform.addEventListener('submit', onSubmit);
    $('#askcancel').addEventListener('click', onCancel);
    askdlg.addEventListener('cancel', onCancel);
    askdlg.showModal(); inp.focus();
  });
}
async function pickModel(p) {
  if (!p.name) return;
  const r = rowOf(p.name);
  if (!r?.agent) return toast(t('model.shell'));
  const a = agents.find(x => x.name === r.agent);
  const m = await ask(t('model.title', { name: p.name }), a?.models?.length ? t('model.eg', { list: a.models.join(', ') }) : t('model.ph'), a?.models || []);
  if (!m) return;
  const res = await api(`/v1/terms/${encodeURIComponent(p.name)}/model`, { method: 'POST', body: { model: m } });
  if (!res.ok) return toast(await res.text());
  toast(t('model.sent', { name: p.name, m }), false);
  p.term?.focus();
}
const handoffdlg = $('#handoffdlg'), hoform = $('#hoform');
let hoFrom = null, hoTo = '';
function paintHoSeg() {
  const cands = agents.filter(a => a.name !== 'shell');
  $('#hoseg').innerHTML = cands.map(a => `<button type="button" data-agent="${esc(a.name)}" class="${a.name === hoTo ? 'on' : ''}">${esc(a.name)}</button>`).join('');
  const a = cands.find(x => x.name === hoTo);
  $('#homodels').innerHTML = (a?.models || []).map(m => `<option value="${esc(m)}">`).join('');
}
function openHandoff(p) {
  if (!p.name) return;
  hoFrom = p;
  const cur = rowOf(p.name)?.agent;
  const cands = agents.filter(a => a.name !== 'shell');
  hoTo = (cands.find(a => a.name !== cur) || cands[0])?.name || '';
  $('#hotitle').textContent = t('ho.title', { name: p.name });
  hoform.reset(); paintHoSeg();
  handoffdlg.showModal();
}
$('#hoclose').addEventListener('click', () => handoffdlg.close());
$('#hoseg').addEventListener('click', e => { const b = e.target.closest('[data-agent]'); if (!b) return; hoTo = b.dataset.agent; paintHoSeg(); });
hoform.addEventListener('submit', async e => {
  e.preventDefault();
  if (!hoFrom?.name || !hoTo) return;
  const f = new FormData(hoform);
  const r = await api(`/v1/terms/${encodeURIComponent(hoFrom.name)}/handoff`, { method: 'POST', body: { to: hoTo, model: f.get('model') || '', close: f.get('close') === 'on' } });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) return toast(j.error || r.statusText);
  handoffdlg.close();
  toast(`${hoFrom.name} → ${j.term.name}. ${j.file.split('/').slice(-2).join('/')}`, false);
  const target = j.closed ? hoFrom : (panes.length < 4 ? addPane() : focusedPane());
  openIn(target, j.term.name);
});

/* ---------- live feed ---------- */
function connectFeed() {
  const es = new EventSource(withToken('/v1/stream'));
  es.addEventListener('snapshot', e => {
    snap = JSON.parse(e.data);
    renderList();
    detectTransitions();
    if (!restored) restoreLayout();
  });
  es.onopen = () => { connEl.textContent = 'live'; connEl.classList.add('on'); };
  es.onerror = () => {
    connEl.textContent = 'reconnecting'; connEl.classList.remove('on');
    if (!getToken()) { es.close(); api('/v1/terms').then(() => connectFeed()).catch(() => {}); }
  };
}
applyI18n();
loadMeta();
connectFeed();
setInterval(renderList, 15000);
