#!/bin/sh
# Check the Go port (windows/engine) against engine/engine.js
set -e
cd "$(dirname "$0")/.."
T=${TMPDIR:-/tmp}/mr-parity; mkdir -p $T
python3 - > $T/inputs.txt <<'PY'
import json,re
d=json.load(open('data/eval/mr_sent.json'))
seen=set()
for s in d:
    for w in re.findall(r'[A-Za-z]+', s['romanized sentence']):
        if w not in seen: seen.add(w); print(w)
for w in ['ek','prem','iitd','sindkhedraja','ratri','aalo','meeting','the','mahar','shivaj','kaL','maLa','ghaDyal','bhaNDaN','Dhol','x','Q','aaaa','constructor','toString','phone','college','station','kal','mla','zala','dnyaneshwar','kshama','Shivaji','a','AEIOU','ksh','gny','nchh']:
    print(w)
PY
(cd windows && go run ./cmd/parity keys ../data/mr-words-full.tsv) > $T/go-keys.txt
node tests/parity.js keys data/mr-words-full.tsv > $T/js-keys.txt
(cd windows && go run ./cmd/parity suggest ../data/mr-lexicon.tsv ../data/en-words.txt) < $T/inputs.txt > $T/go-sug.txt
node tests/parity.js suggest data/mr-lexicon.tsv data/en-words.txt < $T/inputs.txt > $T/js-sug.txt
echo "keys:    $(wc -l < $T/js-keys.txt) words, $(diff $T/js-keys.txt $T/go-keys.txt | grep -c '^<' || true) differences"
echo "suggest: $(wc -l < $T/js-sug.txt) inputs, $(diff $T/js-sug.txt $T/go-sug.txt | grep -c '^<' || true) differences"
diff $T/js-keys.txt $T/go-keys.txt | head -6 || true
diff $T/js-sug.txt $T/go-sug.txt | head -6 || true
