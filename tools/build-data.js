// Compile the frequency list into the lexicon shipped with the extension and
// the Windows app: "word<TAB>log10(freq)<TAB>space-separated lookup keys".
// usage: node tools/build-data.js data/mr-words-full.tsv 300000 data/mr-lexicon.tsv
const fs = require('fs');
const { wordKeys } = require('../engine/engine.js');
const [src, n, dst] = process.argv.slice(2);
const lines = fs.readFileSync(src, 'utf8').split('\n').filter(Boolean).slice(0, +n || 300000);
const out = ['# Marathi lexicon for mr-xlit. Word frequencies: AI4Bharat IndicXlit word_prob_dicts (IndicCorp), MIT licence.'];
for (const l of lines) {
  const [w, p] = l.split('\t');
  const keys = wordKeys(w);
  if (!keys.length) continue;
  out.push(`${w}\t${(Math.round(Math.log10(parseFloat(p)) * 100) / 100)}\t${keys.join(' ')}`);
}
fs.writeFileSync(dst, out.join('\n') + '\n');
console.log(`${out.length - 1} words -> ${dst} (${(fs.statSync(dst).size / 1e6).toFixed(1)} MB)`);
