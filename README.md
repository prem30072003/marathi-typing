# Marathi Typing (मराठी टायपिंग)

Type Marathi in English letters and get Devanagari as you type, the way your phone keyboard does it:

    tumhi kase aahat  →  तुम्ही कसे आहात
    mnje tula nakki kay pahije  →  म्हणजे तुला नक्की काय पाहिजे

There are two front-ends. Both use the same engine and the same offline word list:

| | Works in | File |
|---|---|---|
| **Chrome extension** | Gmail, WhatsApp Web, Google Docs, forms, most websites (Chrome, Edge, Brave) | `marathi-typing-extension.zip` |
| **Windows app** | Every program: MS Word, Excel, PowerPoint, Outlook, Notepad, WhatsApp desktop, browsers | `MarathiTyping.exe` |

Nothing is sent to the internet. All the work happens on your computer.

## What's new in 0.2

- **Always-visible mode.** Windows gets a floating **म / EN** badge: drag it anywhere, click it to switch, right-click it for all settings. The browser shows a small म / EN chip in the corner of the text box. The Windows tray icon now uses a real icon, comes back if Explorer restarts, and the first-run message explains where Windows 11 hides tray icons.
- **Smarter suggestions.**
  - Word completion: `mahar` also offers महाराष्ट्र…, marked with "…".
  - Context: it learns which word you usually type after which.
  - Common English words are offered unchanged as the 2nd choice, or kept in English automatically if you turn that setting on. This catches about 81% of English words and wrongly flags about 0.7% of Marathi words, measured on code-mixed chat data.
- **My dictionary.** Add your own shortcuts (`addr = full address`) and words (names, places).
  - Browser: Settings & my dictionary, with import and export.
  - Windows: right-click the badge → Edit my words…. It opens in Notepad and reloads when you save.
- **Marathi digits** (optional): `2026` → २०२६.

## Install

**Chrome extension**
1. Unzip `marathi-typing-extension.zip`.
2. Open `chrome://extensions`, turn on **Developer mode** (top right), click **Load unpacked** and pick the unzipped folder.
3. Click into any text box and type. **Alt+M** turns Marathi on and off.

**Windows app**
1. Double-click `MarathiTyping.exe`. A **म** icon appears in the system tray, near the clock.
2. Windows may say *"Windows protected your PC"*, because the app is not code-signed yet. Click **More info → Run anyway**.
3. Type in Word or any other program. **Alt+M** switches between Marathi and English. Right-click the tray icon to set **Start with Windows**.

## Keys (same in both)

| Key | What it does |
|---|---|
| Space | types the highlighted word, then a space |
| Enter / Tab | types the highlighted word |
| 1 – 7, or a click | picks a different suggestion |
| ↑ ↓ | moves the highlight |
| Esc | keeps what you typed in English letters |
| Alt+M | switches between Marathi and English |
| Capital T D N L | asks for ट ड ण ळ, for example `paaNi` → पाणी, `baL` → बाळ |

The engine remembers any word you pick with 1–7 and suggests it first the next time.

## How it works

`engine/engine.js` is the engine. `windows/engine/engine.go` is a line-by-line port of it.

1. **Lexicon.** It holds the 300,000 most frequent Marathi words, with frequencies from IndicCorp (AI4Bharat IndicXlit `word_prob_dicts`).
2. **Indexing.** Rules romanize each word the way people actually type it. They handle schwa deletion (करतो → *karto*), झ as *z*, व as *w/v*, ज्ञ as *dny/gy*, ं as *n/m*, and so on. Each word is stored under a loose phonetic key, so *mhanje*, *mhnje* and *mhanaje* all reach म्हणजे.
3. **Ranking.** Words that share the typed key are ranked by frequency and by how closely their spelling matches what you typed.
4. **Extras.** A shortcut table handles chat spellings (*mla, kr, nhi, pn, mnje*). Another table handles English loanwords (*phone* → फोन, *college* → कॉलेज). Long words that aren't in the list get split into two known words (*anuvadshastrat* → अनुवादशास्त्रात). If nothing else matches, a rule-based speller writes the word out.

**Accuracy.** Tested on 4,617 human-romanized Marathi sentences (AI4Bharat/Dakshina, split into dev and held-out test halves):

| | Top-1 | Top-5 |
|---|---|---|
| Held-out test half | 83.0% | 87.6% |
| Everyday chat sentences (`tests/chat.js`, a smoke test I wrote myself) | 67 / 67 words | |

Most of the misses are proper nouns and English words spelled the English way.

**How each front-end types**

- **Extension.** Letters are held in a small suggestion box and don't reach the page yet. On Space, the word is inserted with `execCommand('insertText')`, which keeps undo working and works with React, WhatsApp Web and Gmail. Google Docs gets a synthetic paste event instead.
- **Windows app.** A low-level keyboard hook (`WH_KEYBOARD_LL`) holds the letters. The chosen word is then typed into the active program with `SendInput` Unicode events, the same way a real keyboard types. That's why it works in Office without any add-in.

## Build from source

```sh
python3 tools/prepare-data.py                      # downloads the word frequencies and evaluation data
node tools/build-data.js data/mr-words-full.tsv 300000 data/mr-lexicon.tsv
./tools/build-extension.sh                          # -> dist/marathi-typing-extension.zip
./tools/build-windows.sh                            # -> dist/MarathiTyping.exe  (needs Go 1.22+)
node tests/eval.js data/mr-lexicon.tsv test         # accuracy
node tests/chat.js                                  # chat smoke test
./tests/parity.sh                                   # checks that the Go port gives identical output to the JS engine
```

## Known limits

- **Google Docs.** It works through a paste event and is best-effort. If it ever misbehaves, Docs has its own Marathi input tools: Ctrl+Alt+Shift+K.
- **Admin programs.** The Windows app can't type into programs running as Administrator unless it also runs as Administrator.
- **Passwords.** In browsers, turn the tool off (Alt+M) before typing a password. The Windows app skips classic Windows password boxes automatically.
- **Next steps.**
  - Code-sign the `.exe` so SmartScreen stops warning.
  - Publish to the Chrome Web Store.
  - Add a macOS version (same engine).
  - Add a small neural model (IndicXlit, MIT) for rare words.

## Credits and licences

- Word frequencies: AI4Bharat IndicXlit, `word_prob_dicts` (MIT).
- Evaluation sentences: Google Dakshina via AI4Bharat (CC BY-SA 4.0). Used only for testing and not shipped.
- English word list: wordfreq by Robyn Speer (CC BY-SA 4.0), filtered (`data/en-words.txt`).
- English-detection check: L3Cube MeLID (CC BY-NC-SA 4.0). Used only for testing and not shipped.
- Icons are generated with Noto Sans Devanagari (SIL OFL).
