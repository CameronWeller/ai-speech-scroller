#!/usr/bin/env bash
# Run on the main desktop; the prompter goes fullscreen on the named monitor (default HDMI-1).
exec "$(dirname "$0")/../ai-speech-scroller" -monitor "${1:-HDMI-1}"
