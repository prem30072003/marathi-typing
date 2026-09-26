const $ = (id) => document.getElementById(id);
let host = '';

async function init() {
  const { enabled = true, disabledSites = [] } = await chrome.storage.local.get(['enabled', 'disabledSites']);
  $('on').checked = enabled;
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    host = tab && tab.url ? new URL(tab.url).hostname : '';
  } catch (e) { host = ''; }
  $('host').textContent = host ? '(' + host + ')' : '';
  $('site').checked = !disabledSites.includes(host);
  $('site').disabled = !host;
  stats();
}

async function stats() {
  const r = await chrome.runtime.sendMessage({ type: 'stats' }).catch(() => null);
  if (r) $('stats').textContent = `${r.learned + r.context} things learned`;
}

$('on').addEventListener('change', (e) => chrome.storage.local.set({ enabled: e.target.checked }));
$('site').addEventListener('change', async (e) => {
  const { disabledSites = [] } = await chrome.storage.local.get('disabledSites');
  const set = new Set(disabledSites);
  if (e.target.checked) set.delete(host); else set.add(host);
  chrome.storage.local.set({ disabledSites: [...set] });
});
$('settings').addEventListener('click', () => chrome.runtime.openOptionsPage());
init();
$('try').focus();
