#!/bin/sh
# Build MarathiTyping.exe (cross-compiles from Linux/macOS/Windows; needs Go 1.22+).
set -e
cd "$(dirname "$0")/.."
gzip -9 -c data/mr-lexicon.tsv > windows/cmd/marathityping/lexicon.tsv.gz
cp data/en-words.txt windows/cmd/marathityping/en-words.txt
cp windows/assets/mr-on.ico windows/assets/mr-off.ico windows/cmd/marathityping/
mkdir -p dist
cd windows
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o ../dist/MarathiTyping.exe ./cmd/marathityping
ls -la ../dist/MarathiTyping.exe
