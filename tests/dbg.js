const { chromium } = require('playwright');
const path = require('path');
(async () => {
  const ext = path.resolve(__dirname, '../extension');
  const ctx = await chromium.launchPersistentContext('', { headless: false, args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`] });
  const page = await ctx.newPage();
  page.on('console', m => console.log('console:', m.text()));
  await page.goto('http://localhost:8765/index.html');
  await page.waitForTimeout(800);
  await page.click('#ta');
  for (const w of ['tu ', 'kay ', 'kartoys ', 'mala ', 'khup ', 'bhuk ', 'lagli ', 'ahe ']) {
    await page.keyboard.type(w, { delay: 25 });
    await page.waitForTimeout(50);
    console.log(JSON.stringify(await page.evaluate(() => document.getElementById('ta').value)));
  }
  await ctx.close();
})();
