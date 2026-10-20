#!/usr/bin/env bash
# Fetch libvosk (Apache-2.0) into lib/ and the small English model (Apache-2.0) into
# internal/asr/model.tar.gz, which the binary embeds.
set -euo pipefail
cd "$(dirname "$0")/.."
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
if [ ! -f lib/libvosk.so ]; then
  mkdir -p lib
  curl -fsSL -o "$tmp/vosk.zip" https://github.com/alphacep/vosk-api/releases/download/v0.3.45/vosk-linux-x86_64-0.3.45.zip
  unzip -q -j "$tmp/vosk.zip" '*/libvosk.so' -d lib
fi
if [ ! -f internal/asr/model.tar.gz ]; then
  curl -fsSL -o "$tmp/m.zip" https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip
  (cd "$tmp" && unzip -q m.zip && tar czf "$OLDPWD/internal/asr/model.tar.gz" vosk-model-small-en-us-0.15)
fi
ls -lh lib internal/asr/model.tar.gz
