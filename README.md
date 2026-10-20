# AI Speech Scroller

A teleprompter that follows your voice. Speech recognition runs **offline**, and the tracker is
**misread-tolerant**: if the recogniser drops or garbles a word or two, the script keeps moving as
soon as it hears another word it recognises further along. It never jumps backward, and a lone
"the" or an unrelated word far ahead can't yank the script away.

One static binary (Go, no cgo) with the UI, the Vosk speech engine and an English model embedded.

## Use

```bash
./ai-speech-scroller -monitor HDMI-1     # prompter fullscreen on that monitor (Linux/xrandr)
./ai-speech-scroller -display-at 10240,0 # or by screen position (any OS)
./ai-speech-scroller -no-open            # just serve; open /display.html yourself
```

The control page opens on your main screen: paste a script, pick a microphone, **Load script**,
**Start listening**. Size, width, reading-line position and mirror (for beam-splitter glass) apply live.
Use ◀/▶ 5 words to nudge the position. Needs Chrome or Chromium for the display window.

## Build

```bash
scripts/fetch-assets.sh        # vosk-browser + vosk-model-small-en-us-0.15 -> web/vendor/ (embedded)
go build -o ai-speech-scroller .
npm test                       # matcher tests (Node, no dependencies)
```

Releases: pushing a `v*` tag builds Linux/macOS/Windows binaries and `.deb` packages
(`nfpm.yaml`) and attaches them to a GitHub release. A hosted apt repository for `apt upgrade`
is future work; the `.deb` is the groundwork.

## How the tracking works

`web/matcher.js` keeps the next expected word. For each recognition hypothesis it looks at the
last few heard words and searches a window ahead (12 words). A nearby match (≤3 words ahead)
moves the script; a farther one needs two consecutive words to line up. Matching is exact,
prefix (partial results) or fuzzy (edit similarity ≥ 0.72).

## Licence

MIT. Vosk and the model are Apache-2.0.
