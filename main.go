// ai-speech-scroller: a native teleprompter that follows your voice.
//
//	ai-speech-scroller [flags] script.txt
//
// Esc opens the menu. Mic capture and offline recognition run inside this process.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/CameronWeller/ai-speech-scroller/internal/asr"
	"github.com/CameronWeller/ai-speech-scroller/internal/match"
)

var version = "dev"

var (
	colBG       = sdl.Color{R: 11, G: 13, B: 16, A: 255}
	colUpcoming = sdl.Color{R: 242, G: 242, B: 242, A: 255}
	colDone     = sdl.Color{R: 91, G: 100, B: 112, A: 255}
	colNow      = sdl.Color{R: 255, G: 210, B: 74, A: 255}
	colCue      = sdl.Color{R: 70, G: 80, B: 92, A: 255}
	colAccent   = sdl.Color{R: 255, G: 210, B: 74, A: 255}
)

type placed struct {
	tok, x, y int
	cue       bool
}

type app struct {
	cfg  Config
	path string
	mt   time.Time

	win *sdl.Window
	r   *sdl.Renderer
	gl  *glyphs
	eng *asr.Engine

	mu      sync.Mutex // guards m, heard, status
	m       *match.Matcher
	heard   string
	status  string
	pending string // script path chosen in a file dialog, picked up by the main loop

	micIdx    int // -1 = system default
	dispIdx   int
	listening bool

	menuOpen bool
	keysOpen bool
	menuSel  int
	menuSub  int
	hits     []hit
	mx, my   int32 // mouse position in window pixels
	lastMove time.Time

	words      []placed
	lineH      int
	laidW      int
	laidSize   int
	laidCol    int
	laidTokens int

	offset, target float64
	target2        *sdl.Texture // offscreen frame, flipped when mirroring
	tw, th         int32
	quit           bool
}

func main() {
	runtime.LockOSThread()
	listOnly := flag.Bool("list", false, "list displays and microphones, then exit")
	display := flag.String("display", "", "screen to use: index or part of its name (remembered)")
	mic := flag.String("mic", "", "microphone: part of its name (remembered)")
	windowed := flag.Bool("windowed", false, "run in a normal window instead of fullscreen")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	a := &app{cfg: loadConfig(), micIdx: -1}
	if flag.NArg() > 0 {
		a.cfg.Script = flag.Arg(0)
	}
	a.path = a.cfg.Script
	if *display != "" {
		a.cfg.Display = *display
	}
	if *mic != "" {
		a.cfg.Mic = *mic
	}

	if os.Getenv("SDL_VIDEODRIVER") == "" {
		os.Setenv("SDL_VIDEODRIVER", "wayland,x11")
	}
	sdl.SetHint(sdl.HINT_VIDEO_MINIMIZE_ON_FOCUS_LOSS, "0")
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_EVENTS); err != nil {
		fatal(err)
	}
	defer sdl.Quit()

	var err error
	if a.eng, err = asr.New(); err != nil {
		fatal(err)
	}
	if *listOnly {
		a.list()
		return
	}
	a.pickDisplay()
	a.pickMic()
	a.setup(*windowed)
	a.loadScript()
	if !a.cfg.NoListen {
		a.startListening()
	}
	a.loop()
	a.eng.Stop()
	a.cfg.save()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ai-speech-scroller:", err)
	os.Exit(1)
}

func (a *app) list() {
	n, _ := sdl.GetNumVideoDisplays()
	fmt.Println("Displays:")
	for i := 0; i < n; i++ {
		name, _ := sdl.GetDisplayName(i)
		b, _ := sdl.GetDisplayBounds(i)
		fmt.Printf("  %d  %s  %dx%d\n", i, name, b.W, b.H)
	}
	fmt.Println("Microphones:")
	for _, m := range a.eng.Mics() {
		fmt.Println("  ", m.Name)
	}
}

func displayName(i int) string {
	n, _ := sdl.GetDisplayName(i)
	return n
}

func (a *app) pickDisplay() {
	n, _ := sdl.GetNumVideoDisplays()
	a.dispIdx = 0
	want := a.cfg.Display
	if want == "" {
		return
	}
	for i := 0; i < n; i++ {
		if fmt.Sprint(i) == want || strings.Contains(strings.ToLower(displayName(i)), strings.ToLower(want)) {
			a.dispIdx = i
			a.cfg.Display = displayName(i)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "display %q not found; using %s\n", want, displayName(0))
}

func (a *app) pickMic() {
	a.micIdx = -1
	if a.cfg.Mic == "" {
		return
	}
	for i, m := range a.eng.Mics() {
		if strings.Contains(strings.ToLower(m.Name), strings.ToLower(a.cfg.Mic)) {
			a.micIdx = i
			a.cfg.Mic = m.Name
			return
		}
	}
	fmt.Fprintf(os.Stderr, "microphone %q not found; using the system default\n", a.cfg.Mic)
}

func (a *app) setup(windowed bool) {
	pos := int32(sdl.WINDOWPOS_CENTERED_MASK | uint32(a.dispIdx))
	flags := uint32(sdl.WINDOW_SHOWN | sdl.WINDOW_ALLOW_HIGHDPI)
	if windowed {
		flags |= sdl.WINDOW_RESIZABLE
	} else {
		flags |= sdl.WINDOW_FULLSCREEN_DESKTOP
	}
	var err error
	if a.win, err = sdl.CreateWindow("AI Speech Scroller", pos, pos, 1280, 720, flags); err != nil {
		fatal(err)
	}
	if a.r, err = sdl.CreateRenderer(a.win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC|sdl.RENDERER_TARGETTEXTURE); err != nil {
		fatal(err)
	}
	sdl.DisableScreenSaver()
	a.gl = newGlyphs(a.r)
}

func (a *app) moveToDisplay(i int) {
	a.dispIdx = i
	a.cfg.Display = displayName(i)
	pos := int32(sdl.WINDOWPOS_CENTERED_MASK | uint32(i))
	_ = a.win.SetFullscreen(0)
	a.win.SetPosition(pos, pos)
	_ = a.win.SetFullscreen(sdl.WINDOW_FULLSCREEN_DESKTOP)
}

const welcome = "No script loaded.\n\nRun:  ai-speech-scroller script.txt\n\nEsc opens the menu."

func (a *app) loadScript() {
	text := welcome
	if a.path != "" {
		st, err := os.Stat(a.path)
		b, rerr := os.ReadFile(a.path)
		if err != nil || rerr != nil {
			text = "Cannot read " + a.path
		} else {
			text, a.mt = string(b), st.ModTime()
		}
	}
	a.mu.Lock()
	old := 0
	if a.m != nil {
		old = a.m.Pos()
	}
	a.m = match.New(text)
	a.m.Reset(old)
	a.mu.Unlock()
	a.laidTokens = -1
}

func (a *app) startListening() {
	err := a.eng.Start(a.micIdx, func(w []string, final bool) {
		a.mu.Lock()
		a.m.Feed(w)
		a.heard = strings.Join(w, " ")
		a.mu.Unlock()
	})
	a.mu.Lock()
	if err != nil {
		a.status = err.Error()
	} else {
		a.status = ""
	}
	a.mu.Unlock()
	a.listening = err == nil
	a.cfg.NoListen = !a.listening
}

func (a *app) stopListening() {
	a.eng.Stop()
	a.listening = false
	a.cfg.NoListen = true
}

func (a *app) toggleListening() {
	if a.listening {
		a.stopListening()
	} else {
		a.startListening()
	}
}

func (a *app) nudge(n int) {
	a.mu.Lock()
	a.m.Reset(a.m.Pos() + n)
	a.mu.Unlock()
}

func (a *app) cycleMic(d int) {
	n := len(a.eng.Mics()) + 1 // +1 for "system default" at index -1
	a.micIdx = (a.micIdx+1+d+n)%n - 1
	if a.micIdx >= 0 {
		a.cfg.Mic = a.eng.Mics()[a.micIdx].Name
	} else {
		a.cfg.Mic = ""
	}
	if a.listening {
		a.startListening()
	}
}

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

func (a *app) key(k sdl.Keycode) {
	if a.keysOpen { // any key closes the key sheet
		a.keysOpen = false
		return
	}
	if k == sdl.K_F1 || k == sdl.K_QUESTION {
		a.keysOpen = true
		return
	}
	if k == sdl.K_ESCAPE {
		a.menuOpen = !a.menuOpen
		a.cfg.save()
		return
	}
	if a.menuOpen {
		a.menuKey(k)
		return
	}
	switch k {
	case sdl.K_v:
		if sdl.GetModState()&sdl.KMOD_CTRL != 0 {
			a.pasteScript()
		}
	case sdl.K_o:
		if sdl.GetModState()&sdl.KMOD_CTRL != 0 {
			a.openScript()
		}
	case sdl.K_SPACE:
		a.toggleListening()
	case sdl.K_LEFT:
		a.nudge(-5)
	case sdl.K_RIGHT:
		a.nudge(5)
	case sdl.K_UP:
		a.nudge(-1)
	case sdl.K_DOWN:
		a.nudge(1)
	case sdl.K_HOME:
		a.nudge(-1 << 30)
	case sdl.K_m:
		a.cfg.Mirror = !a.cfg.Mirror
	case sdl.K_EQUALS, sdl.K_PLUS, sdl.K_KP_PLUS:
		a.cfg.Size = clamp(a.cfg.Size+4, 24, 240)
	case sdl.K_MINUS, sdl.K_KP_MINUS:
		a.cfg.Size = clamp(a.cfg.Size-4, 24, 240)
	}
}

// layout wraps the script into a column and records each word's document position.
func (a *app) layout(w int) {
	a.mu.Lock()
	toks := a.m.Tokens
	a.mu.Unlock()
	if w == a.laidW && a.cfg.Size == a.laidSize && a.cfg.Column == a.laidCol && len(toks) == a.laidTokens {
		return
	}
	a.laidW, a.laidSize, a.laidCol, a.laidTokens = w, a.cfg.Size, a.cfg.Column, len(toks)
	a.gl.clear()
	main := faceKey{px: a.cfg.Size}
	cue := faceKey{px: max(14, a.cfg.Size/2), italic: true}
	a.lineH = a.gl.lineHeight(main) * 135 / 100
	colW := w * a.cfg.Column / 100
	x0 := (w - colW) / 2
	space := a.gl.measure(main, " ")
	x, y := 0, 0
	a.words = a.words[:0]
	for i, t := range toks {
		k := main
		if t.Cue {
			k = cue
		}
		tw := a.gl.measure(k, t.Raw)
		if i > 0 && t.Br > 0 {
			x, y = 0, y+a.lineH
			if t.Br > 1 {
				y += a.lineH / 2
			}
		} else if x > 0 && x+tw > colW {
			x, y = 0, y+a.lineH
		}
		a.words = append(a.words, placed{tok: i, x: x0 + x, y: y, cue: t.Cue})
		x += tw + space
	}
}

func (a *app) frame(dt float64) {
	w, h, _ := a.r.GetOutputSize()
	if a.target2 == nil || a.tw != w || a.th != h {
		if a.target2 != nil {
			a.target2.Destroy()
		}
		a.target2, _ = a.r.CreateTexture(sdl.PIXELFORMAT_RGBA8888, sdl.TEXTUREACCESS_TARGET, w, h)
		a.tw, a.th = w, h
	}
	a.layout(int(w))
	a.mu.Lock()
	tp := a.m.TokenPos()
	heard, status := a.heard, a.status
	toks := a.m.Tokens
	a.mu.Unlock()
	if len(a.words) > 0 {
		tp = min(tp, len(a.words)-1)
		a.target = float64(a.words[tp].y+a.lineH/2) - float64(h)*float64(a.cfg.Guide)/100
		a.target = math.Max(a.target, -float64(h)*float64(a.cfg.Guide)/100)
	}
	a.offset += (a.target - a.offset) * (1 - math.Exp(-dt*6))

	_ = a.r.SetRenderTarget(a.target2)
	a.r.SetDrawColor(colBG.R, colBG.G, colBG.B, 255)
	a.r.Clear()
	main := faceKey{px: a.cfg.Size}
	cue := faceKey{px: max(14, a.cfg.Size/2), italic: true}
	cur := a.curTok()
	for _, p := range a.words {
		y := p.y - int(a.offset)
		if y > int(h) || y+a.lineH < 0 {
			continue
		}
		k, c := main, colUpcoming
		switch {
		case p.cue:
			k, c = cue, colCue
		case p.tok < cur:
			c = colDone
		case p.tok == cur:
			c = colNow
		}
		a.gl.draw(k, toks[p.tok].Raw, p.x, y, c)
	}
	gy := int32(float64(h) * float64(a.cfg.Guide) / 100)
	a.r.SetDrawColor(255, 210, 74, 40)
	a.r.DrawLine(0, gy, w, gy)
	a.hits = a.hits[:0]
	if a.keysOpen {
		a.drawKeys(w, h)
	} else if a.menuOpen {
		a.drawMenu(w, h, heard, status)
	} else if a.mouseActive() {
		a.drawBar(w, h)
	}
	a.dot(w)
	_ = a.r.SetRenderTarget(nil)
	flip := sdl.FLIP_NONE
	if a.cfg.Mirror {
		flip = sdl.FLIP_HORIZONTAL
	}
	_ = a.r.CopyEx(a.target2, nil, nil, 0, nil, flip)
	// Use the system cursor; hide it while the prompter is untouched.
	if a.menuOpen || a.keysOpen || a.mouseActive() {
		sdl.ShowCursor(sdl.ENABLE)
	} else {
		sdl.ShowCursor(sdl.DISABLE)
	}
	a.r.Present()
}

func (a *app) curTok() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.m.TokenPos()
}

func (a *app) loop() {
	last := time.Now()
	lastPoll := time.Now()
	for !a.quit {
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			switch e := ev.(type) {
			case *sdl.QuitEvent:
				a.quit = true
			case *sdl.MouseMotionEvent:
				a.mouseAt(e.X, e.Y)
			case *sdl.MouseButtonEvent:
				a.mouseAt(e.X, e.Y)
				if e.Type == sdl.MOUSEBUTTONDOWN && e.Button == sdl.BUTTON_LEFT {
					a.click()
				}
			case *sdl.MouseWheelEvent:
				a.lastMove = time.Now()
				if !a.menuOpen {
					a.nudge(-int(e.Y))
				}
			case *sdl.KeyboardEvent:
				if e.Type == sdl.KEYDOWN {
					a.key(e.Keysym.Sym)
				}
			}
		}
		a.mu.Lock()
		p := a.pending
		a.pending = ""
		a.mu.Unlock()
		if p != "" {
			a.useScript(p)
		}
		if time.Since(lastPoll) > time.Second {
			lastPoll = time.Now()
			if st, err := os.Stat(a.path); a.path != "" && err == nil && st.ModTime() != a.mt {
				a.loadScript()
			}
		}
		now := time.Now()
		a.frame(now.Sub(last).Seconds())
		last = now
	}
}

// mouseAt records the mouse position in output pixels (window units can differ on HiDPI).
func (a *app) mouseAt(x, y int32) {
	ww, wh := a.win.GetSize()
	ow, oh, _ := a.r.GetOutputSize()
	a.mx, a.my, a.lastMove = x*ow/max(ww, 1), y*oh/max(wh, 1), time.Now()
}

// pasteScript replaces the script with the clipboard text (saved so it survives restarts and reloads).
func (a *app) pasteScript() {
	text, err := sdl.GetClipboardText()
	if err != nil || strings.TrimSpace(text) == "" {
		a.setStatus("Clipboard is empty")
		return
	}
	dir := filepath.Dir(configPath())
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "pasted.txt")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		a.setStatus(err.Error())
		return
	}
	a.useScript(p)
	a.setStatus("Pasted script loaded")
}

// openScript asks the desktop for a file via zenity or kdialog.
func (a *app) openScript() {
	var cmd *exec.Cmd
	if p, err := exec.LookPath("zenity"); err == nil {
		cmd = exec.Command(p, "--file-selection", "--title=Open script")
	} else if p, err := exec.LookPath("kdialog"); err == nil {
		cmd = exec.Command(p, "--getopenfilename")
	} else {
		a.setStatus("No file dialog found (install zenity), or pass the file on the command line")
		return
	}
	go func() { // keep the prompter responsive while the dialog is open
		out, err := cmd.Output()
		if p := strings.TrimSpace(string(out)); err == nil && p != "" {
			a.mu.Lock()
			a.pending = p
			a.mu.Unlock()
		}
	}()
}

func (a *app) useScript(p string) {
	a.path = p
	a.cfg.Script = p
	a.cfg.save()
	a.loadScript()
	a.nudge(-1 << 30)
}

func (a *app) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
}
