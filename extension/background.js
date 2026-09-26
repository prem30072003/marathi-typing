/* Service worker: owns the engine + lexicon, answers suggestion requests. */
importScripts('engine.js');

const DEFAULT_DICT = `# My dictionary – one entry per line
#   shortcut = what it should type
#   or just a Marathi word you use often (so loose spellings find it)
# Examples:
# gm = शुभ सकाळ!
# addr = १२, शिवाजी नगर, पुणे ४११००५
# कोल्हटकर
`;

let engine = null;
let loading = null;
let saveTimer = null;

function getEngine() {
  if (engine) return Promise.resolve(engine);
  if (!loading) {
    loading = (async () => {
      const [lex, en] = await Promise.all([
        fetch(chrome.runtime.getURL('data/mr-lexicon.tsv')).then(r => r.text()),
        fetch(chrome.runtime.getURL('data/en-words.txt')).then(r => r.text()),
      ]);
      const e = new MarathiEngine.Engine().load(lex).loadEnglish(en);
      const { learned, userDict, opts = {} } = await chrome.storage.local.get(['learned', 'userDict', 'opts']);
      if (learned) e.importLearned(learned);
      e.setUserDict(userDict == null ? DEFAULT_DICT : userDict);
      e.setOptions({ keepEnglish: !!opts.keepEnglish });
      engine = e;
      return e;
    })();
  }
  return loading;
}

function scheduleSave() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    if (engine) chrome.storage.local.set({ learned: engine.exportLearned() });
  }, 1500);
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  switch (msg.type) {
    case 'suggest':
      getEngine().then(e => {
        const list = e.suggest(msg.input, 5, msg.prev || '');
        const comp = e.complete(msg.input, 2, list);
        sendResponse({ id: msg.id, list, comp });
      });
      return true;
    case 'warm':
      getEngine();
      return false;
    case 'learn':
      getEngine().then(e => {
        if (msg.picked) e.learn(msg.input, msg.word);
        e.learnContext(msg.prev, msg.word);
        scheduleSave();
      });
      return false;
    case 'forget':
      getEngine().then(e => { e.importLearned({}); chrome.storage.local.set({ learned: '{}' }); sendResponse({ ok: true }); });
      return true;
    case 'stats':
      getEngine().then(e => sendResponse({
        words: e.words.length, learned: Object.keys(e.learned).length,
        context: Object.keys(e.bigram).length, mine: e.uwords.length + Object.keys(e.user).length,
      }));
      return true;
    case 'getDict':
      chrome.storage.local.get('userDict').then(({ userDict }) => sendResponse({ text: userDict == null ? DEFAULT_DICT : userDict }));
      return true;
    case 'setDict':
      chrome.storage.local.set({ userDict: msg.text }).then(() => getEngine()).then(e => sendResponse({ n: e.setUserDict(msg.text) }));
      return true;
  }
  return false;
});

async function setBadge() {
  const { enabled = true } = await chrome.storage.local.get('enabled');
  chrome.action.setBadgeText({ text: enabled ? 'म' : '' });
  chrome.action.setBadgeBackgroundColor({ color: '#c2410c' });
  chrome.action.setTitle({ title: enabled ? 'Marathi typing: ON (Alt+M to turn off)' : 'Marathi typing: OFF (Alt+M to turn on)' });
}

chrome.commands.onCommand.addListener(async (cmd) => {
  if (cmd !== 'toggle-marathi') return;
  const { enabled = true } = await chrome.storage.local.get('enabled');
  await chrome.storage.local.set({ enabled: !enabled });
});

chrome.storage.onChanged.addListener((c) => {
  if (c.enabled) setBadge();
  if (c.opts && engine) engine.setOptions({ keepEnglish: !!(c.opts.newValue || {}).keepEnglish });
});
chrome.runtime.onInstalled.addListener((d) => {
  setBadge();
  if (d.reason === 'install') chrome.tabs.create({ url: 'options.html?welcome=1' });
});
chrome.runtime.onStartup.addListener(setBadge);
