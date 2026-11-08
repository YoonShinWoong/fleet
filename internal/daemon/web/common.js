// Shared by index and term pages: token handling and an authed fetch.
const TOKEN_KEY = 'fleet.token';
function getToken() { try { return localStorage.getItem(TOKEN_KEY) || ''; } catch { return ''; } }
function setToken(t) { try { localStorage.setItem(TOKEN_KEY, t); } catch {} }
function withToken(url) {
  const t = getToken(); if (!t) return url;
  return url + (url.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(t);
}
async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {});
  if (opts.body && typeof opts.body !== 'string') { opts.body = JSON.stringify(opts.body); headers['content-type'] = 'application/json'; }
  const t = getToken(); if (t) headers['authorization'] = 'Bearer ' + t;
  let r = await fetch(path, Object.assign({}, opts, { headers }));
  if (r.status === 401) {
    const tok = prompt('fleet token (from ~/.fleet/config.json on the host)');
    if (!tok) throw new Error('unauthorized');
    setToken(tok.trim());
    return api(path, opts);
  }
  return r;
}
let toastTimer;
function toast(msg, isError = true) {
  const t = document.getElementById('toast'); if (!t) return alert(msg);
  t.textContent = msg; t.hidden = false; t.style.background = isError ? '#ef4444' : '#16a34a';
  clearTimeout(toastTimer); toastTimer = setTimeout(() => t.hidden = true, 4000);
}
function age(ts) {
  const s = Math.max(0, (Date.now() - new Date(ts)) / 1000);
  if (s < 60) return `${Math.floor(s)}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}
function esc(t) { return String(t ?? '').replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }
