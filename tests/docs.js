const { chromium } = require('playwright');
const path = require('path');
(async () => {
  const ext = path.resolve(__dirname, '../extension');
  const ctx = await chromium.launchPersistentContext('', { headless: false, args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`] });
  const page = await ctx.newPage();
  await page.goto('http://localhost:8765/docs.html');
  await page.waitForTimeout(1500);
  await page.frameLocator('iframe').locator('body').click({ force: true }).catch(e => console.log('click err', e.message));
  await page.frameLocator('iframe').locator('body').focus();
  await page.keyboard.type('ganpati bappa mory', { delay: 30 });
  await page.waitForTimeout(400);
  await page.screenshot({ path: '/tmp/claude-0/docs.png' });
  await page.keyboard.type('a ', { delay: 30 });
  await page.waitForTimeout(400);
  console.log(await page.evaluate(() => ({ docs: document.getElementById('docsout').textContent, leaked: window.leaked || '' })));
  await ctx.close();
})();
