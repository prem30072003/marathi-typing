// v2 features: my dictionary, completions, Marathi digits, keep-English, context, chip, options page
const { chromium } = require('playwright');
const path = require('path');
(async () => {
  const ext = path.resolve(__dirname, '../extension');
  const ctx = await chromium.launchPersistentContext('', { headless: false, args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`] });
  let [sw] = ctx.serviceWorkers(); if (!sw) sw = await ctx.waitForEvent('serviceworker');
  const id = sw.url().split('/')[2];
  const errors = [];
  await new Promise(r => setTimeout(r, 1000));
  // options page: save a dictionary and turn on digits
  const opt = ctx.pages().find(p => p.url().includes('options.html')) || await ctx.newPage();
  opt.on('pageerror', e => errors.push('options: ' + e.message));
  if (!opt.url().includes('options.html')) await opt.goto(`chrome-extension://${id}/options.html?welcome=1`);
  await opt.setViewportSize({ width: 900, height: 1300 });
  await opt.waitForTimeout(800);
  await opt.fill('#dict', 'ek = एक समाज\nsindkhed = सिंदखेड राजा\nप्रेमकुमार\n');
  await opt.click('#save'); await opt.waitForTimeout(300);
  await opt.click('#digits + span'); await opt.waitForTimeout(300);
  console.log('dict status:', await opt.textContent('#dictStatus'));
  await opt.click('.try'); await opt.keyboard.type('maza naav premkumar ahe ', { delay: 30 }); await opt.waitForTimeout(300);
  console.log('welcome try box:', await opt.inputValue('.try'));
  await opt.screenshot({ path: '/tmp/claude-0/options.png' });

  const page = await ctx.newPage();
  page.on('pageerror', e => errors.push(e.message));
  await page.goto('http://localhost:8765/index.html'); await page.waitForTimeout(500);
  await page.click('#ta');
  await page.waitForTimeout(300);
  const chip = await page.evaluate(() => { const h = document.querySelector('mr-xlit-popup'); const c = h && h.shadowRoot.querySelector('.chip'); return c ? c.textContent : null; });
  await page.keyboard.type('ek ', { delay: 30 });
  await page.keyboard.type('sindkhed ', { delay: 30 });
  await page.keyboard.type('2026 ', { delay: 30 });
  await page.keyboard.type('mahar', { delay: 30 }); await page.waitForTimeout(250);
  await page.screenshot({ path: '/tmp/claude-0/completion.png' });
  const items = await page.evaluate(() => [...document.querySelector('mr-xlit-popup').shadowRoot.querySelectorAll('li')].map(l => l.textContent));
  // pick the first completion (after 5 suggestions it is number 6 at most)
  const compIdx = items.findIndex(t => t.includes('…'));
  await page.keyboard.press(String(compIdx + 1)); await page.keyboard.type(' ', { delay: 30 });
  console.log('textarea:', JSON.stringify(await page.inputValue('#ta')));
  console.log('chip:', chip, '| popup items for mahar:', items.join(' / '));
  // context: teach "mi" -> "aalo" once via picking, then check ranking uses prev word
  await page.click('#inp');
  for (let i = 0; i < 2; i++) { await page.keyboard.type('kal ratri ', { delay: 20 }); }
  await page.waitForTimeout(300);
  const r = await page.evaluate(() => chrome.runtime ? 'n/a' : 'n/a');
  // keep-English option
  await opt.bringToFront(); await opt.click('#keepEnglish + span'); await opt.waitForTimeout(300);
  await page.bringToFront(); await page.click('#ce'); await page.keyboard.type('meeting la ye ', { delay: 30 }); await page.waitForTimeout(300);
  console.log('keep English ->', await page.evaluate(() => document.getElementById('ce').innerText));
  console.log('errors:', errors);
  await ctx.close();
})();
