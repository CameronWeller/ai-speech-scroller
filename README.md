# AI Speech Scroller

A native teleprompter that follows your voice. Microphone capture and speech recognition run
**offline inside the app** (Vosk), and the tracker is **misread-tolerant**: if the recogniser drops
or garbles a word or two, the script keeps moving as soon as it hears another word it recognises
further along. It never jumps backward, and a lone "the" or an unrelated word far ahead can't yank
the script away. No browser involved.

## Use

```bash
ai-speech-scroller script.txt              # opens fullscreen on the screen you last chose
ai-speech-scroller -list                   # show screens and microphones
ai-speech-scroller -display HJW script.txt # pick a screen by index or part of its name (remembered)
ai-speech-scroller -mic "USB Audio" script.txt
```

Change the script from the menu (**Script: Paste clipboard / Open file...**) or with `Ctrl+V` (paste) and `Ctrl+O` (open). Editing the script file while it runs reloads it. `[bracketed]` text and lines starting with `→`
are stage directions: shown dim, never expected to be spoken.

**Esc opens the menu**: listening on/off, microphone, screen, text size, column width, reading
line, mirror (off by default; on for glass that reflects the screen), restart, nudge, quit. Use ↑↓ to select,
←→ to change, Enter to activate. With the menu closed: Space pauses/resumes listening, ←/→ nudge
five words, ↑/↓ one word, Home restarts, `M` mirrors, `+`/`-` text size. Settings are remembered in
`~/.config/ai-speech-scroller/config.json`. A dot in the top-right corner is green while listening.

## Build

Linux (Wayland or X11), Go 1.25, SDL2 and ALSA/Pulse headers:

```bash
sudo apt install libsdl2-dev libasound2-dev
scripts/fetch-assets.sh     # libvosk.so -> lib/, model -> internal/asr/model.tar.gz (embedded)
go build -o ai-speech-scroller .
go test ./...
```

`libvosk.so` is loaded from next to the binary, `lib/`, or `/usr/lib/ai-speech-scroller`.
Releases: pushing a `v*` tag builds a Linux tarball and a `.deb` (`nfpm.yaml`) and attaches them
to a GitHub release. A hosted apt repository for `apt upgrade` is future work.

## How the tracking works

`internal/match` keeps the next expected word. For each recognition hypothesis it looks at the
last few heard words and searches 12 words ahead. A match within 3 words moves the script; a
farther one needs two consecutive words to line up. Matching is exact, prefix (partial results)
or fuzzy (edit similarity ≥ 0.72). `cmd/wavtest` runs a WAV file through the whole pipeline.

## Licence

MIT. Vosk and the model are Apache-2.0.
