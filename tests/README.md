Tests
- `node tests/eval.js [lexicon] [dev|test|all]` – accuracy on human-romanised sentences
- `node tests/chat.js` – chat-style smoke test
- `./tests/parity.sh` – Go port vs JS engine, must print 0 differences
- Browser tests (need Playwright + a display, e.g. `xvfb-run`): serve `tests/site` on port 8765
  (`cd tests/site && python3 -m http.server 8765`), then `node tests/e2e.js` (inputs, textarea,
  contenteditable, password field, Alt+M) and `node tests/docs.js` (Google-Docs-style hidden iframe).
