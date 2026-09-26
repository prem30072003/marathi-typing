const fs = require('fs');
const M = require('../engine/engine.js');
const userDict = '# test\nek = एक समाज\nprem = प्रेमकुमार\nसिंदखेडराजा\nIITD\tआयआयटी दिल्ली\n bad line =\n= x\n';
const out = [];
if (process.argv[2] === 'keys') {
  for (const l of fs.readFileSync(process.argv[3], 'utf8').split('\n')) {
    if (!l) continue; const w = l.split('\t')[0];
    out.push(`${w}\t${M.wordKeys(w).join(' ')}\t${M.rulesToDev(M.devToRomanVariants(w).join(''))}`);
  }
} else {
  const e = new M.Engine().load(fs.readFileSync(process.argv[3], 'utf8'));
  e.loadEnglish(fs.readFileSync(process.argv[4], 'utf8'));
  out.push(`userdict\t${e.setUserDict(userDict)}`);
  e.learn('kal', 'काळ'); e.learn('constructor', 'X');
  e.learnContext('मी', 'आलो'); e.learnContext('काल', 'रात्री'); e.learnContext('काल', 'रात्री');
  for (const l of fs.readFileSync(0, 'utf8').split('\n')) {
    if (!l) continue;
    const s = e.suggest(l, 7, ''), ctx = e.suggest(l, 7, 'काल'), comp = e.complete(l, 2, s);
    e.setOptions({ keepEnglish: true }); const ke = e.suggest(l, 7, 'मी'); e.setOptions({ keepEnglish: false });
    out.push(`${l}\t${s.join('|')}\t${ctx.join('|')}\t${comp.join('|')}\t${ke.join('|')}`);
  }
}
process.stdout.write(out.join('\n') + '\n');
