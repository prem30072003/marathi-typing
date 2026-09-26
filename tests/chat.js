// Smoke test on everyday chat-style Marathi.  usage: node tests/chat.js
const fs = require('fs'), path = require('path');
const { Engine } = require('../engine/engine.js');
const e = new Engine().load(fs.readFileSync(path.join(__dirname, '../data/mr-lexicon.tsv'), 'utf8'));
const cases = {
  'tu kay kartoys': 'तू काय करतोयस', 'mala mahit nahi': 'मला माहीत नाही', 'mi ghari jato ahe': 'मी घरी जातो आहे',
  'udya aapan bhetuya ka': 'उद्या आपण भेटूया का', 'maza phone kharab zala': 'माझा फोन खराब झाला',
  'mnje tula nakki kay pahije': 'म्हणजे तुला नक्की काय पाहिजे', 'khup chan vatla': 'खूप छान वाटलं',
  'shivaji maharaj ki jay': 'शिवाजी महाराज की जय', 'aamchya gavat jatra ahe': 'आमच्या गावात जत्रा आहे',
  'dnyaneshwar': 'ज्ञानेश्वर', 'kshama kara': 'क्षमा करा', 'ganpati bappa morya': 'गणपती बाप्पा मोरया',
  'pune mumbai nashik kolhapur': 'पुणे मुंबई नाशिक कोल्हापूर', 'tyamule mi nahi aalo': 'त्यामुळे मी नाही आलो',
  'shala sutli': 'शाळा सुटली', 'kiti vajle': 'किती वाजले', 'hya varshi paus changla zala': 'ह्या वर्षी पाऊस चांगला झाला',
  'college la jaycha ahe': 'कॉलेज ला जायचं आहे', 'paaNi pyayla de': 'पाणी प्यायला दे', 'tumhi kase aahat': 'तुम्ही कसे आहात',
};
let ok = 0, n = 0;
for (const [src, want] of Object.entries(cases)) {
  const got = src.split(' ').map(w => e.suggest(w, 5)[0]).join(' ');
  const w1 = src.split(' ').length; n += w1;
  const good = got.split(' ').filter((g, i) => g === want.split(' ')[i]).length; ok += good;
  console.log(good === w1 ? '✓' : '✗', src.padEnd(30), got, good === w1 ? '' : `   (expected ${want})`);
}
console.log(`\n${ok}/${n} words right as the first suggestion`);
