// Shared by index and term pages: token handling and an authed fetch.
const TOKEN_KEY = 'fleet.token';
// A native shell can hand the token over once as ?token=...; keep it and hide it from the URL.
try { const u = new URL(location.href); const qt = u.searchParams.get('token'); if (qt) { localStorage.setItem(TOKEN_KEY, qt); u.searchParams.delete('token'); history.replaceState(null, '', u.pathname + (u.search || '') + u.hash); } } catch {}
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

// ---- i18n: English by default, Korean when the browser (or fleet.lang) says so ----
const LANG = (() => { try { const l = localStorage.getItem('fleet.lang'); if (l) return l; } catch {} return (navigator.language || 'en').toLowerCase().startsWith('ko') ? 'ko' : 'en'; })();
const STR = {
  en: {
    'group.needs_input': 'Needs input', 'group.working': 'Working', 'group.error': 'Error', 'group.idle': 'Idle', 'group.shell': 'Shell', 'group.unknown': 'Unknown', 'group.exited': 'Exited', 'group.ended': 'Ended',
    'summary.waiting1': '1 needs input', 'summary.waiting': '{n} need input', 'summary.working': '{n} working',
    'filter.ph': 'Filter by name · 2+ chars also searches output',
    'hint.switch': 'switch', 'hint.next': 'next waiting', 'hint.split': 'split', 'hint.close': 'close', 'hint.max': 'maximize', 'hint.find': 'find',
    'empty.main': 'Pick a terminal on the left, or start one with + new.', 'list.none': 'Nothing yet. Start with + new.',
    'ws.other': 'Other', 'ws.otherMeta': 'outside workspaces', 'ws.empty': 'No terminals. Add one with +', 'ws.add': 'Add a terminal to this workspace', 'ws.forget': 'Forget this workspace', 'ws.forgetConfirm': 'forget?',
    'count.input': '{n} waiting', 'viewers1': '1 viewer', 'viewers': '{n} viewers',
    'item.split': 'Open in a new pane', 'item.kill': 'kill', 'item.killConfirm': 'kill?', 'item.restored': 'resumed',
    'pane.choose': 'Pick a terminal from the list to open it here.', 'pane.exited': 'terminal exited ({code})',
    'pane.find': 'Find in output (⌥F)', 'pane.model': 'Switch model (types /model)', 'pane.handoff': 'Hand off to another agent', 'pane.max': 'Maximize (⌥M)', 'pane.close': 'Close (⌥W)',
    'search.ph': 'Find in output. Enter next, Shift+Enter previous, Esc closes', 'search.group': 'Found in output · {n}', 'search.terms1': '1 terminal', 'search.terms': '{n} terminals', 'search.none': 'none', 'search.addon': 'Could not load the search addon (CDN)', 'search.prev': 'previous', 'search.next': 'next', 'search.close': 'close',
    'toast.max': 'Up to 4 panes',
    'dlg.title': 'New terminal', 'dlg.workspace': 'Workspace', 'dlg.newWs': '+ new workspace', 'dlg.wsname': 'Workspace name', 'dlg.wsname.ph': 'e.g. fix-login', 'dlg.where': 'Where', 'dlg.pathOpt': 'Enter a folder path', 'dlg.repoOpt': '{name} · new worktree', 'dlg.path': 'Folder path', 'dlg.branch': 'Branch (optional)', 'dlg.branch.ph': 'default: workspace name', 'dlg.agent': 'Agent', 'dlg.model': 'Model (optional)', 'dlg.model.ph': 'default model', 'dlg.model.eg': 'default model (e.g. {list})', 'dlg.name': 'Terminal name (optional)', 'dlg.name.ph': 'auto: workspace-agent', 'dlg.start': 'Start', 'dlg.shell': 'shell',
    'ask.ok': 'OK',
    'model.title': '{name}: switch model', 'model.ph': 'model name', 'model.eg': 'e.g. {list}', 'model.sent': '{name}: model command sent ({m})', 'model.shell': 'Shells have no model. Use this on an agent terminal.',
    'ho.title': '{name} → another agent', 'ho.to': 'Hand off to', 'ho.model': 'Model (optional)', 'ho.close': 'Close the old terminal afterwards', 'ho.note': 'Writes the old terminal’s last output and git status to .fleet/handoff-*.md, then starts the new agent with a first prompt that points at it.', 'ho.submit': 'Hand off',
    'notify.needs': 'needs your input', 'notify.done': 'turn finished', 'notify.exited': 'terminal exited ({code})', 'notify.error': 'error', 'notify.none': 'No terminal is waiting for input', 'notify.unsupported': 'This browser does not support notifications', 'notify.denied': 'Notification permission denied',
    'btn.new': '+ new', 'btn.bell': 'Desktop notifications', 'btn.sound': 'Sound', 'back': 'list',
  },
  ko: {
    'group.needs_input': '입력 필요', 'group.working': '작업 중', 'group.error': '오류', 'group.idle': '대기', 'group.shell': '셸', 'group.unknown': '알 수 없음', 'group.exited': '종료', 'group.ended': '종료',
    'summary.waiting1': '1 입력 필요', 'summary.waiting': '{n} 입력 필요', 'summary.working': '{n} 작업 중',
    'filter.ph': '이름 필터 · 두 글자부터는 출력도 검색',
    'hint.switch': '전환', 'hint.next': '다음 입력 대기', 'hint.split': '분할', 'hint.close': '닫기', 'hint.max': '최대화', 'hint.find': '찾기',
    'empty.main': '왼쪽에서 터미널을 고르거나 + new 로 만들어.', 'list.none': '아직 없음. + new 로 시작해.',
    'ws.other': '기타', 'ws.otherMeta': '워크스페이스 밖', 'ws.empty': '터미널 없음. + 로 추가', 'ws.add': '이 워크스페이스에 터미널 추가', 'ws.forget': '워크스페이스 잊기', 'ws.forgetConfirm': '잊기?',
    'count.input': '{n} 입력', 'viewers1': '1 viewer', 'viewers': '{n} viewers',
    'item.split': '옆 칸으로 열기', 'item.kill': 'kill', 'item.killConfirm': 'kill?', 'item.restored': '복구됨',
    'pane.choose': '왼쪽 목록에서 터미널을 고르면 이 칸에 열려.', 'pane.exited': '터미널 종료 ({code})',
    'pane.find': '출력에서 찾기 (⌥F)', 'pane.model': '모델 바꾸기 (/model 입력)', 'pane.handoff': '다른 에이전트로 넘기기', 'pane.max': '최대화 (⌥M)', 'pane.close': '닫기 (⌥W)',
    'search.ph': '출력에서 찾기. Enter 다음, Shift+Enter 이전, Esc 닫기', 'search.group': '출력에서 찾음 · {n}', 'search.terms1': '1개 터미널', 'search.terms': '{n}개 터미널', 'search.none': '없음', 'search.addon': '검색 애드온을 못 불러왔어 (CDN)', 'search.prev': '이전', 'search.next': '다음', 'search.close': '닫기',
    'toast.max': '칸은 최대 4개까지',
    'dlg.title': '새 터미널', 'dlg.workspace': '워크스페이스', 'dlg.newWs': '+ 새 워크스페이스', 'dlg.wsname': '워크스페이스 이름', 'dlg.wsname.ph': '예: fix-login', 'dlg.where': '어디서', 'dlg.pathOpt': '폴더 경로 직접 입력', 'dlg.repoOpt': '{name} · 새 워크트리', 'dlg.path': '폴더 경로', 'dlg.branch': '브랜치 (선택)', 'dlg.branch.ph': '기본: 워크스페이스 이름', 'dlg.agent': '에이전트', 'dlg.model': '모델 (선택)', 'dlg.model.ph': '기본 모델', 'dlg.model.eg': '기본 모델 (예: {list})', 'dlg.name': '터미널 이름 (선택)', 'dlg.name.ph': '자동: 워크스페이스-에이전트', 'dlg.start': '시작', 'dlg.shell': '셸',
    'ask.ok': '확인',
    'model.title': '{name} 모델 바꾸기', 'model.ph': '모델 이름', 'model.eg': '예: {list}', 'model.sent': '{name}: 모델 명령 보냄 ({m})', 'model.shell': '셸 터미널에는 모델이 없어. 에이전트 터미널에서 써.',
    'ho.title': '{name} → 다른 에이전트로', 'ho.to': '받을 에이전트', 'ho.model': '모델 (선택)', 'ho.close': '넘긴 뒤 이전 터미널 닫기', 'ho.note': '이전 터미널의 마지막 출력과 git 상태를 .fleet/handoff-*.md 로 남기고, 새 에이전트가 그 파일을 읽고 이어가도록 첫 프롬프트를 넣어 시작해.', 'ho.submit': '넘기기',
    'notify.needs': '입력이 필요해', 'notify.done': '턴 끝남', 'notify.exited': '터미널 종료 ({code})', 'notify.error': '오류', 'notify.none': '입력 기다리는 터미널 없음', 'notify.unsupported': '이 브라우저는 알림을 지원하지 않아', 'notify.denied': '알림 권한이 없어',
    'btn.new': '+ new', 'btn.bell': '데스크톱 알림', 'btn.sound': '소리', 'back': '목록',
  },
};
function t(key, vars) {
  let s = (STR[LANG] && STR[LANG][key]) ?? STR.en[key] ?? key;
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.split('{' + k + '}').join(v);
  return s;
}
function applyI18n(root = document) {
  root.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
  root.querySelectorAll('[data-i18n-ph]').forEach(el => { el.placeholder = t(el.dataset.i18nPh); });
  root.querySelectorAll('[data-i18n-title]').forEach(el => { el.title = t(el.dataset.i18nTitle); });
  document.documentElement.lang = LANG;
}
