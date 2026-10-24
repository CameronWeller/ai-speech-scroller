# AI Speech Scroller

A native teleprompter that follows your voice. Microphone capture and speech recognition run
**offline inside the app** ([Vosk](https://alphacephei.com/vosk/)); nothing leaves your machine and
no browser is involved.

The tracker is **misread-tolerant**. If the recogniser drops or garbles a word or two, the script
keeps moving as soon as it hears another word it recognises further along. It never jumps backward,
and a lone "the" or an unrelated word far ahead can't yank the script away.

Built for a hardware teleprompter on a spare monitor (developed against a Neewer X11 on its own
display), but it works fullscreen on any screen.

## Install

### apt (Ubuntu/Debian, auto-updating)

```bash
sudo curl -fsSL https://cameronweller.github.io/ai-speech-scroller/ai-speech-scroller.asc \
  -o /etc/apt/keyrings/ai-speech-scroller.asc
echo "deb [signed-by=/etc/apt/keyrings/ai-speech-scroller.asc] https://cameronweller.github.io/ai-speech-scroller stable main" \
  | sudo tee /etc/apt/sources.list.d/ai-speech-scroller.list
sudo apt update && sudo apt install ai-speech-scroller
```

### Flatpak

```bash
flatpak remote-add --user --if-not-exists ai-speech-scroller \
  https://cameronweller.github.io/ai-speech-scroller/flatpak/ai-speech-scroller.flatpakrepo
flatpak install --user ai-speech-scroller io.github.CameronWeller.AiSpeechScroller
flatpak run io.github.CameronWeller.AiSpeechScroller
```

Scripts are readable from Documents and Downloads inside the sandbox; in-app edits to those are
saved as a copy in the app's config directory.

### Direct download

The [releases page](https://github.com/CameronWeller/ai-speech-scroller/releases) has the `.deb`,
a `.flatpak` bundle, and a tarball (keep `libvosk.so` next to the binary; needs `libsdl2-2.0-0`).

Linux x86-64 only for now (Wayland or X11).

## Quick start

```bash
ai-speech-scroller -list                    # screens and microphones
ai-speech-scroller -display HJW script.txt  # fullscreen on that screen; remembered next time
```

Then press **Space** to start or stop listening and read. **Esc** opens the menu.

| Flag | Meaning |
|---|---|
| `script.txt` | script to show (remembered; omit to reuse the last one) |
| `-display X` | screen by index or part of its name (remembered) |
| `-mic X` | microphone by part of its name (remembered) |
| `-windowed` | normal window instead of fullscreen |
| `-list` | list screens and microphones, then exit |

## Scripts

Plain text. Blank lines are kept as paragraph gaps. Stage directions are shown dim and are never
expected to be spoken: anything in `[brackets]` and any line starting with `→` or `->`.

Change the script without leaving the prompter: **Ctrl+E** edits it in place, **Ctrl+V** pastes the
clipboard, **Ctrl+O** opens a file (needs `zenity` or `kdialog`), and editing the file on disk
reloads it within a second.

### Editing in the app

**Ctrl+E** (or the **Edit** button, or Menu → Script → Edit...) opens the script in a built-in
editor with the cursor on the word you were reading. Saving writes the script file and reloads the
prompter without losing your place.

| Key | Action |
|---|---|
| Ctrl+S | save |
| Ctrl+Enter | save and close |
| Esc | close (with unsaved changes, press it twice to discard them) |
| Ctrl+Z / Ctrl+Shift+Z or Ctrl+Y | undo / redo |
| Ctrl+A, Ctrl+C, Ctrl+X, Ctrl+V | select all, copy, cut, paste |
| Shift + arrows / Home / End | select |
| Ctrl + ← → / Backspace / Delete | move or delete by word |
| Ctrl+Home / Ctrl+End | start / end of the script |
| Ctrl + / Ctrl − | editor text size |

Click places the cursor, drag selects, double-click selects a word and triple-click a line. Line
endings (`\n` or `\r\n`) are kept. If the file changed on disk while you were editing, the first
save warns and a second one overwrites it. If the file cannot be written (for example, Documents and
Downloads are read-only in the Flatpak), the edit is saved to `edited.txt` in the config directory
and the prompter switches to that copy.

## Controls

Every key action is also a menu item or an on-screen button. Move the mouse and a button bar
appears; **F1** (or Menu → Help) shows a visual key sheet.

| Key | Action |
|---|---|
| Esc | open / close the menu |
| Space | pause / resume listening |
| ← → | back / forward 5 words |
| ↑ ↓ | back / forward 1 word |
| Home | restart the script |
| Mouse wheel | nudge the position |
| + / − | text size |
| M | mirror (for glass that reflects the screen; off by default) |
| Ctrl+E | edit the script |
| Ctrl+V / Ctrl+O | paste / open a script |
| F1 or ? | key sheet |

The menu also sets microphone, screen, text size, column width and reading-line position.
Settings persist in `~/.config/ai-speech-scroller/config.json`. A small square in the top-right
corner is green while listening and grey when paused.

## Troubleshooting

- **Wrong screen**: `-list`, then `-display <name or index>`, or Esc → Screen.
- **No sound picked up**: Esc → Microphone and pick the right input; loopback "Monitor of …"
  sources are hidden.
- **Backwards text**: toggle Mirror (M).
- **Skips or stalls**: speak the script roughly in order; one or two misreads are fine, but a long
  ad-lib needs ← / → to re-sync.

## Build from source

Needs Go 1.25, `libsdl2-dev` and `libasound2-dev`.

```bash
scripts/fetch-assets.sh   # libvosk.so -> lib/, small English model -> internal/asr/model.tar.gz (embedded)
go build -o ai-speech-scroller .
go test ./...
```

`libvosk.so` is loaded from beside the binary, `lib/`, or `/usr/lib/ai-speech-scroller`.
Pushing a `v*` tag builds the tarball and `.deb` (`nfpm.yaml`) and attaches them to a GitHub release.

## How the tracking works

`internal/match` keeps the next expected word. For each recognition result it looks at the last few
heard words and searches 12 words ahead. A match within 3 words moves the script; a farther one needs
two consecutive words to line up. Matching is exact, prefix (partial results cut words short) or
fuzzy (edit similarity ≥ 0.72). `internal/asr` captures audio with miniaudio and feeds Vosk;
`cmd/wavtest` runs a WAV file through the whole pipeline.

## Licence

MIT. Vosk and its model are Apache-2.0.
