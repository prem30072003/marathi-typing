const { chromium } = require('playwright');
const path = require('path');
(async () => {
  const ext = path.resolve(__dirname, '../extension');
  const ctx = await chromium.launchPersistentContext('', { headless: false, args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`] });
  let [sw] = ctx.serviceWorkers(); if (!sw) sw = await ctx.waitForEvent('serviceworker');
  const id = sw.url().split('/')[2];
  const errors = [];
  const page = await ctx.newPage();
  page.on('pageerror', e => errors.push(e.message)); page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  await page.setViewportSize({ width: 300, height: 420 });
  await page.goto(`chrome-extension://${id}/popup.html`);
  await page.waitForTimeout(1200);
  await page.click('#try'); await page.keyboard.type('tumhi kase aahat? maza naav', { delay: 30 });
  await page.waitForTimeout(400);
  await page.screenshot({ path: '/tmp/claude-0/extpopup.png' });
  console.log('try box:', await page.inputValue('#try'), '| stats:', await page.textContent('#stats'), '| errors:', errors);
  await ctx.close();
})();
