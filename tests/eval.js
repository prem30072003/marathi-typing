// Word-level accuracy on AI4Bharat/Dakshina human-romanised Marathi sentences.
// usage: node tests/eval.js [wordsFile] [split: dev|test|all]
const fs = require('fs');
const path = require('path');
const { Engine } = require('../engine/engine.js');

const wordsFile = process.argv[2] || path.join(__dirname, '../data/mr-lexicon.tsv');
const split = process.argv[3] || 'all';
const sents = JSON.parse(fs.readFileSync(path.join(__dirname, '../data/eval/mr_sent.json'), 'utf8'));

const t0 = Date.now();
const eng = new Engine().load(fs.readFileSync(wordsFile, 'utf8'));
const loadMs = Date.now() - t0;

const devRe = /[ऀ-ॿ]+/g, romRe = /[A-Za-z]+/g;
let pairs = [];
sents.forEach((s, i) => {
  if (split === 'dev' && i % 2) return;
  if (split === 'test' && !(i % 2)) return;
  const d = s['native sentence'].match(devRe) || [];
  const r = s['romanized sentence'].match(romRe) || [];
  if (d.length && d.length === r.length) d.forEach((w, j) => pairs.push([r[j], w]));
});

let top1 = 0, top3 = 0, top5 = 0, n = 0, inLex = 0;
const lex = new Set(eng.words);
const misses = [];
const t1 = Date.now();
for (const [r, w] of pairs) {
  n++;
  if (lex.has(w)) inLex++;
  const s = eng.suggest(r, 6);
  const pos = s.indexOf(w);
  if (pos === 0) top1++;
  if (pos >= 0 && pos < 3) top3++;
  if (pos >= 0 && pos < 5) top5++;
  if (pos !== 0 && misses.length < 4000) misses.push([r, w, s.slice(0, 3).join(' '), lex.has(w)]);
}
const ms = Date.now() - t1;
const pct = x => (100 * x / n).toFixed(1) + '%';
console.log(`words=${n} lexicon=${eng.words.length} load=${loadMs}ms  avg=${(ms / n).toFixed(2)}ms/word`);
console.log(`top1=${pct(top1)} top3=${pct(top3)} top5=${pct(top5)}  target-in-lexicon=${pct(inLex)}`);
if (process.env.MISSES) {
  const m = misses.filter(x => x[3]);
  for (let i = 0; i < m.length; i += Math.ceil(m.length / +process.env.MISSES)) console.log(m[i].join(' | '));
}
