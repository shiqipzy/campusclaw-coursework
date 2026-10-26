const $ = (id) => document.getElementById(id);
let currentUser = null;
let selectedID = null;
let accessToken = sessionStorage.getItem('campusclaw_token');

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (accessToken && path !== '/api/login') headers.set('Authorization', `Bearer ${accessToken}`);
  const response = await fetch(path, { ...options, headers });
  if (response.status === 401 && path !== '/api/login') {
    accessToken = null;
    sessionStorage.removeItem('campusclaw_token');
    showLogin();
    throw new Error('请重新登录');
  }
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || `请求失败 (${response.status})`);
  }
  return response.status === 204 ? null : response.json();
}
function showLogin() {
  currentUser = null;
  $('workspace').hidden = true;
  $('login').hidden = false;
  $('password').value = '';
  $('search-query').value = '';
  $('search-results').replaceChildren();
  $('search-message').textContent = '';
  $('ask-answer').textContent = '';
  $('ask-sources').replaceChildren();
}
function showWorkspace(user) {
  currentUser = user;
  $('login').hidden = true;
  $('workspace').hidden = false;
  $('identity').textContent = `${user.username} · ${user.class_name} · ${user.role === 'teacher' ? '教师' : '学生'}`;
  $('upload-panel').hidden = user.role !== 'teacher';
}
async function refreshList() {
  const materials = await api('/api/materials');
  $('count').textContent = materials.length;
  $('list').replaceChildren();
  for (const m of materials) {
    const button = document.createElement('button');
    button.className = 'item' + (selectedID === m.id ? ' active' : '');
    button.dataset.id = m.id;
    const title = document.createElement('strong'); title.textContent = m.title;
    const note = document.createElement('small'); note.textContent = `${m.filename} · ${new Date(m.created_at).toLocaleDateString('zh-CN')}`;
    button.append(title, note);
    button.onclick = () => openMaterial(m.id);
    $('list').append(button);
  }
  if (materials.length && !materials.some((m) => m.id === selectedID)) await openMaterial(materials[0].id);
}
async function openMaterial(id) {
  const m = await api(`/api/materials/${id}`);
  selectedID = id;
  $('empty').hidden = true;
  $('detail').hidden = false;
  $('title').textContent = m.title;
  $('meta').textContent = `${m.filename} · ${new Date(m.created_at).toLocaleString('zh-CN')}`;
  $('content').textContent = m.content;
  $('download').dataset.id = id;
  for (const el of document.querySelectorAll('.item')) el.classList.toggle('active', Number(el.dataset.id) === id);
}
function showSearchResults(data) {
  $('search-results').replaceChildren();
  $('search-message').textContent = data.hits.length ? `找到 ${data.hits.length} 个相关切片` : data.message;
  for (const hit of data.hits) {
    const button = document.createElement('button');
    button.className = 'search-hit';
    button.type = 'button';
    const title = document.createElement('strong'); title.textContent = hit.title;
    const source = document.createElement('small'); source.textContent = `切片 ${hit.chunk_index} · 字符 ${hit.char_start}–${hit.char_end}`;
    const quote = document.createElement('span'); quote.textContent = hit.excerpt;
    button.append(title, source, quote);
    button.onclick = () => openMaterial(hit.material_id).catch((err) => { $('search-message').textContent = err.message; });
    $('search-results').append(button);
  }
}
$('search-form').onsubmit = async (event) => {
  event.preventDefault();
  const button = event.target.querySelector('button'); button.disabled = true;
  $('search-message').textContent = '检索中…';
  $('search-results').replaceChildren();
  try { showSearchResults(await api(`/api/search?q=${encodeURIComponent($('search-query').value)}`)); }
  catch (err) { $('search-message').textContent = err.message; }
  finally { button.disabled = false; }
};
$('ask-form').onsubmit = async (event) => {
  event.preventDefault();
  const button = event.target.querySelector('button'); button.disabled = true;
  $('ask-answer').textContent = '正在查找资料并生成回答…';
  $('ask-sources').replaceChildren();
  try {
    const data = await api('/api/ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ question: $('ask-question').value }) });
    $('ask-answer').textContent = data.answer;
    for (const source of data.sources) {
      const link = document.createElement('button'); link.type = 'button'; link.className = 'source-link';
      link.textContent = `${source.title} · 切片 ${source.chunk_index}`;
      link.onclick = () => openMaterial(source.material_id).catch((err) => { $('ask-answer').textContent = err.message; });
      $('ask-sources').append(link);
    }
  } catch (err) { $('ask-answer').textContent = err.message; }
  finally { button.disabled = false; }
};
$('login-form').onsubmit = async (event) => {
  event.preventDefault();
  $('login-error').textContent = '';
  const button = event.target.querySelector('button'); button.disabled = true;
  try {
    const result = await api('/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: $('username').value, password: $('password').value }) });
    accessToken = result.token;
    sessionStorage.setItem('campusclaw_token', accessToken);
    showWorkspace(result.user); await refreshList();
  } catch (err) { $('login-error').textContent = err.message; }
  finally { button.disabled = false; }
};
$('logout').onclick = async () => {
  try { await api('/api/logout', { method: 'POST' }); accessToken = null; sessionStorage.removeItem('campusclaw_token'); selectedID = null; showLogin(); }
  catch (err) { window.alert(err.message); }
};
$('upload-form').onsubmit = async (event) => {
  event.preventDefault();
  const file = $('file').files[0]; if (!file) return;
  $('upload-message').textContent = '上传中…';
  const button = event.target.querySelector('button'); button.disabled = true;
  try {
    const form = new FormData(); form.append('file', file);
    const m = await api('/api/materials', { method: 'POST', body: form });
    selectedID = m.id; $('upload-message').textContent = '上传成功，正文已入库。';
    await refreshList(); await openMaterial(m.id); $('file').value = '';
  } catch (err) { $('upload-message').textContent = err.message; }
  finally { button.disabled = false; }
};
$('download').onclick = async (event) => {
  event.preventDefault();
  try {
    const response = await fetch(`/api/materials/${event.currentTarget.dataset.id}/file`, { headers: { Authorization: `Bearer ${accessToken}` } });
    if (!response.ok) throw new Error(`下载失败 (${response.status})`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a'); link.href = url; link.download = $('meta').textContent.split(' · ')[0]; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (err) { window.alert(err.message); }
};
(async () => { if (!accessToken) { showLogin(); return; } try { const user = await api('/api/me'); showWorkspace(user); await refreshList(); } catch (err) { showLogin(); } })();
