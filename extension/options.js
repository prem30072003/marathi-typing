const $ = (id) => document.getElementById(id);
const DEFAULT_OPTS = { keepEnglish: false, digits: false, indicator: true };

if (location.search.includes('welcome')) $('welcome').style.display = 'block';

async function init() {
  const { enabled = true, opts = {} } = await chrome.storage.local.get(['enabled', 'opts']);
  const o = Object.assign({}, DEFAULT_OPTS, opts);
  $('enabled').checked = enabled;
  for (const k of Object.keys(DEFAULT_OPTS)) $(k).checked = !!o[k];
  const r = await chrome.runtime.sendMessage({ type: 'getDict' });
  $('dict').value = (r && r.text) || '';
  stats();
}

async function stats() {
  const r = await chrome.runtime.sendMessage({ type: 'stats' }).catch(() => null);
  if (r) $('stats').textContent = `${r.words.toLocaleString()} words in the word list · ${r.mine} entries in my dictionary · ${r.learned} words picked · ${r.context} word pairs learned for context.`;
}

$('enabled').addEventListener('change', (e) => chrome.storage.local.set({ enabled: e.target.checked }));
for (const k of Object.keys(DEFAULT_OPTS)) {
  $(k).addEventListener('change', async () => {
    const o = {};
    for (const kk of Object.keys(DEFAULT_OPTS)) o[kk] = $(kk).checked;
    await chrome.storage.local.set({ opts: o });
  });
}

async function saveDict(text) {
  const r = await chrome.runtime.sendMessage({ type: 'setDict', text });
  $('dictStatus').textContent = `Saved · ${r ? r.n : 0} entries`;
  stats();
}
$('save').addEventListener('click', () => saveDict($('dict').value));
$('dict').addEventListener('keydown', (e) => { if ((e.ctrlKey || e.metaKey) && e.key === 's') { e.preventDefault(); saveDict($('dict').value); } });
$('export').addEventListener('click', () => {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([$('dict').value], { type: 'text/plain;charset=utf-8' }));
  a.download = 'my-marathi-words.txt';
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
});
$('import').addEventListener('click', () => $('file').click());
$('file').addEventListener('change', async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  const text = await f.text();
  $('dict').value = ($('dict').value.trim() ? $('dict').value.replace(/\s*$/, '\n') : '') + text;
  await saveDict($('dict').value);
  e.target.value = '';
});
$('forget').addEventListener('click', async () => { await chrome.runtime.sendMessage({ type: 'forget' }); stats(); });

init();
