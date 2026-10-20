#!/usr/bin/env bash
# Fetch the offline speech engine (vosk-browser, Apache-2.0) and the small English
# model (vosk-model-small-en-us-0.15, Apache-2.0) into web/vendor/ so they embed in the binary.
set -euo pipefail
cd "$(dirname "$0")/.."
V=web/vendor; mkdir -p "$V"
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
if [ ! -f "$V/vosk.js" ]; then
  (cd "$tmp" && npm pack vosk-browser@0.0.8 --silent && tar xzf vosk-browser-*.tgz)
  cp "$tmp/package/dist/vosk.js" "$V/vosk.js"
fi
if [ ! -f "$V/model.tar.gz" ]; then
  curl -fsSL -o "$tmp/m.zip" https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip
  (cd "$tmp" && unzip -q m.zip && tar czf "$OLDPWD/$V/model.tar.gz" vosk-model-small-en-us-0.15)
fi
ls -lh "$V"
