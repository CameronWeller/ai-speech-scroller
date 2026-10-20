#!/usr/bin/env bash
# Run the prompter on the Neewer teleprompter display (matched by name) with the given script.
exec "$(dirname "$0")/../ai-speech-scroller" -display "${DISPLAY_MATCH:-HJW}" "$@"
