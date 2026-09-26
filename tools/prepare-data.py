#!/usr/bin/env python3
"""Download the source data and rebuild data/mr-words-full.tsv (+ the eval set).

Sources (both from the AI4Bharat IndicXlit v1.0 GitHub release):
  word_prob_dicts.zip  - Marathi word frequencies from IndicCorp (MIT licence)
  transliteration-sentence-pairs.json - human-romanised Dakshina sentences,
      used only for evaluation (CC BY-SA 4.0), saved to data/eval/mr_sent.json
Then run:  node tools/build-data.js data/mr-words-full.tsv 300000 data/mr-lexicon.tsv
"""
import io, json, os, re, urllib.request, zipfile

REL = 'https://github.com/AI4Bharat/IndicXlit/releases/download/v1.0/'
here = os.path.dirname(os.path.abspath(__file__))
data = os.path.join(here, '..', 'data')
os.makedirs(os.path.join(data, 'eval'), exist_ok=True)

print('downloading word frequencies (~1 GB zip)...')
z = zipfile.ZipFile(io.BytesIO(urllib.request.urlopen(REL + 'word_prob_dicts.zip').read()))
probs = json.loads(z.read('word_prob_dicts/mr_word_prob_dict.json'))

ok = re.compile(r'^[ऀ-ॣॱ-ॿ]+$')
# malformed spellings that occur in web text (two vowel signs in a row, stray nukta, ...)
bad = re.compile(r'[ा-ौॢॣ][ा-्ॢॣ]|^[ँ-ः़-्]|'
                 r'[ँंः][़-ौ]|््|़़|[^क-ह]़|'
                 r'[ा-ौ]ः|[ँ-ः][ँ-ः]|[अ-औ][ा-्]|्$|'
                 r'़(?<![कखगजडढफ]़)|[अ-औॲ]ः')
single_ok = {'व', 'न', 'आ', 'ई', 'ऊ', 'ए', 'ओ'}
words = sorted(((w, p) for w, p in probs.items()
                if ok.match(w) and not bad.search(w) and len(w) <= 24 and (len(w) > 1 or w in single_ok)),
               key=lambda x: -x[1])[:400000]
with open(os.path.join(data, 'mr-words-full.tsv'), 'w', encoding='utf-8') as f:
    for w, p in words:
        f.write(f'{w}\t{p:.3e}\n')
print(len(words), 'words -> data/mr-words-full.tsv')

print('downloading evaluation sentences...')
d = json.loads(urllib.request.urlopen(REL + 'transliteration-sentence-pairs.json').read())['data']
mr = [x for x in d if x['language'] == 'Marathi']
json.dump(mr, open(os.path.join(data, 'eval', 'mr_sent.json'), 'w', encoding='utf-8'), ensure_ascii=False)
print(len(mr), 'Marathi sentence pairs -> data/eval/mr_sent.json')
