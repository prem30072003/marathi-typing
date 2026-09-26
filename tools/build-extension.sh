#!/bin/sh
# Copy the shared engine + lexicon into the extension and zip it.
set -e
cd "$(dirname "$0")/.."
cp engine/engine.js extension/engine.js
mkdir -p extension/data dist
cp data/mr-lexicon.tsv data/en-words.txt extension/data/
rm -f dist/marathi-typing-extension.zip
(cd extension && zip -qr9 ../dist/marathi-typing-extension.zip . -x '*.DS_Store')
ls -la dist/marathi-typing-extension.zip
