const { chromium } = require('playwright');
const path = require('path');
(async () => {
  const ext = path.resolve(__dirname, '../extension');
  const ctx = await chromium.launchPersistentContext('/tmp/claude-0/pwprofile', {
    headless: false, args: [`--disable-extensions-except=${ext}`, `--load-extension=${ext}`],
  });
  const page = await ctx.newPage();
  await page.goto('http://localhost:8765/index.html');
  await page.waitForTimeout(800);
  const type = async (sel, text) => { await page.click(sel); await page.keyboard.type(text, { delay: 25 }); await page.waitForTimeout(250); };

  await type('#inp', 'mala khup bhuk lagli ahe ');
  await type('#ta', 'tu kay kartoys? ');
  // pick the 2nd suggestion with the number key and keep one word in English with Esc
  await page.click('#ta'); await page.keyboard.type('kal', { delay: 25 }); await page.waitForTimeout(150);
  await page.screenshot({ path: '/tmp/claude-0/popup.png' });
  await page.keyboard.press('2'); await page.keyboard.type(' laptop', { delay: 25 }); await page.keyboard.press('Escape');
  await page.keyboard.type(' ghetla.', { delay: 25 }); await page.waitForTimeout(250);
  await type('#ce', 'maza naav Premkumar ahe. ');
  await page.click('#ce'); await page.keyboard.type('pune', { delay: 25 }); await page.keyboard.press('Backspace'); await page.keyboard.type('e', { delay: 25 });
  await page.keyboard.press('Enter'); await page.waitForTimeout(200);
  await type('#pw', 'secret');
  // toggle off with Alt+M then type English
  await page.click('#inp'); await page.keyboard.press('Alt+M'); await page.waitForTimeout(300);
  await page.keyboard.type('hello', { delay: 25 }); await page.keyboard.press('Alt+M'); await page.waitForTimeout(300);

  const r = await page.evaluate(() => ({
    inp: document.getElementById('inp').value, ta: document.getElementById('ta').value,
    ce: document.getElementById('ce').innerText, pw: document.getElementById('pw').value,
  }));
  console.log(JSON.stringify(r, null, 1));
  await ctx.close();
})();
