package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/CameronWeller/ai-speech-scroller/internal/edit"
)

// editor is the in-app script editor. It edits the script file itself; saving
// reloads the prompter and keeps the reading position.
type editor struct {
	buf     *edit.Buffer
	crlf    bool      // file used \r\n line endings; restored on save
	mt      time.Time // file mtime when opened or last saved
	clobber bool      // file changed on disk; the next Ctrl+S overwrites it anyway
	reading int       // rune offset of the reading position when opened

	rows      []edit.Row
	rowsVer   int
	rowsCols  int
	cols      int
	visRows   int
	scroll    int  // first visible row
	follow    bool // scroll to the cursor on the next frame
	goal      int  // column kept while moving up and down; -1 = none
	warned    bool // one discard attempt was refused because of unsaved changes
	x0, y0    int  // text origin in frame pixels, for mouse hit-testing
	adv, rowH int
	lastAct   time.Time
	msg       string
	msgAt     time.Time
}

func (e *editor) flash(s string) { e.msg, e.msgAt = s, time.Now() }

// layout returns the wrapped rows for the current text and width.
func (e *editor) layout() []edit.Row {
	if e.rows == nil || e.rowsVer != e.buf.Version || e.rowsCols != e.cols {
		e.rows, e.rowsVer, e.rowsCols = edit.Wrap(e.buf.Runes(), e.cols), e.buf.Version, e.cols
	}
	return e.rows
}

// confirmDiscard reports whether unsaved changes may be dropped. The first attempt
// only warns; a second one in a row goes ahead.
func (e *editor) confirmDiscard(again string) bool {
	if !e.buf.Modified() || e.warned {
		return true
	}
	e.warned = true
	e.flash("Unsaved changes: Ctrl+S saves, " + again)
	return false
}

func (a *app) openEditor() {
	text := ""
	var mt time.Time
	if a.path != "" {
		if b, err := os.ReadFile(a.path); err == nil {
			text = string(b)
			if st, err := os.Stat(a.path); err == nil {
				mt = st.ModTime()
			}
		}
	}
	e := &editor{
		buf:  edit.New(strings.ReplaceAll(text, "\r\n", "\n")),
		crlf: strings.Contains(text, "\r\n"), mt: mt,
		cols: 80, goal: -1, follow: true, reading: -1, lastAct: time.Now(),
	}
	if text != "" {
		a.mu.Lock()
		if tp := a.m.TokenPos(); tp < len(a.m.Tokens) {
			e.reading = a.m.Tokens[tp].Off
		}
		a.mu.Unlock()
		e.buf.MoveTo(max(e.reading, 0), false)
	}
	a.ed, a.menuOpen, a.keysOpen = e, false, false
	sdl.StartTextInput()
}

func (a *app) closeEditor() {
	a.ed = nil
	sdl.StopTextInput()
}

// saveEditor writes the script back to its file, or to the config directory when
// there is no file or it cannot be written (read-only Flatpak folders).
func (a *app) saveEditor() bool {
	e := a.ed
	text := e.buf.String()
	if e.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	p := a.path
	if p != "" && !e.clobber {
		if st, err := os.Stat(p); err == nil && !e.mt.IsZero() && !st.ModTime().Equal(e.mt) {
			e.clobber = true
			e.flash(filepath.Base(p) + " changed on disk since you opened it. Ctrl+S again overwrites it")
			return false
		}
	}
	var err error
	if p == "" {
		err = os.ErrNotExist
	} else {
		err = os.WriteFile(p, []byte(text), 0o644)
	}
	if err != nil {
		dir := filepath.Dir(configPath())
		_ = os.MkdirAll(dir, 0o755)
		fb := filepath.Join(dir, "edited.txt")
		if err2 := os.WriteFile(fb, []byte(text), 0o644); err2 != nil {
			e.flash("Save failed: " + err2.Error())
			return false
		}
		if p != "" {
			e.flash("Cannot write " + filepath.Base(p) + "; saved a copy to " + tilde(fb))
		} else {
			e.flash("Saved to " + tilde(fb))
		}
		a.path, a.cfg.Script = fb, fb
		a.cfg.save()
	} else {
		e.flash("Saved " + filepath.Base(p))
	}
	e.buf.MarkSaved()
	e.warned, e.clobber = false, false
	a.loadScript() // keeps the reading position
	if st, err := os.Stat(a.path); err == nil {
		e.mt = st.ModTime()
	}
	a.mu.Lock()
	if tp := a.m.TokenPos(); tp < len(a.m.Tokens) {
		e.reading = a.m.Tokens[tp].Off
	}
	a.mu.Unlock()
	return true
}

// tilde shortens a path under the home directory for display.
func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

func (a *app) editKey(k sdl.Keycode, mod uint16) {
	e, b := a.ed, a.ed.buf
	ctrl, shift := mod&sdl.KMOD_CTRL != 0, mod&sdl.KMOD_SHIFT != 0
	e.lastAct = time.Now()
	if k == sdl.K_ESCAPE {
		if e.confirmDiscard("Esc again discards them") {
			a.closeEditor()
		}
		return
	}
	switch k {
	case sdl.K_LSHIFT, sdl.K_RSHIFT, sdl.K_LCTRL, sdl.K_RCTRL, sdl.K_LALT, sdl.K_RALT, sdl.K_LGUI, sdl.K_RGUI:
		return // modifiers alone change nothing
	}
	e.warned = false
	if k != sdl.K_UP && k != sdl.K_DOWN && k != sdl.K_PAGEUP && k != sdl.K_PAGEDOWN {
		e.goal = -1
	}
	e.follow = true
	if ctrl {
		switch k {
		case sdl.K_s:
			a.saveEditor()
		case sdl.K_RETURN, sdl.K_KP_ENTER:
			if a.saveEditor() {
				a.closeEditor()
			}
		case sdl.K_z:
			if shift {
				b.Redo()
			} else {
				b.Undo()
			}
		case sdl.K_y:
			b.Redo()
		case sdl.K_a:
			b.SelectAll()
		case sdl.K_c, sdl.K_x:
			if s := b.Selected(); s != "" {
				_ = sdl.SetClipboardText(s)
				if k == sdl.K_x {
					b.DeleteSelection()
				}
			}
		case sdl.K_v:
			if s, err := sdl.GetClipboardText(); err == nil {
				b.Insert(s)
			}
		case sdl.K_EQUALS, sdl.K_PLUS, sdl.K_KP_PLUS:
			a.cfg.EditSize = clamp(a.cfg.EditSize+4, 16, 120)
		case sdl.K_MINUS, sdl.K_KP_MINUS:
			a.cfg.EditSize = clamp(a.cfg.EditSize-4, 16, 120)
		}
	}
	lo, hi := b.Selection()
	switch k {
	case sdl.K_LEFT:
		switch {
		case lo != hi && !shift:
			b.MoveTo(lo, false)
		case ctrl:
			b.MoveTo(b.WordLeft(b.Cur), shift)
		default:
			b.MoveTo(b.Cur-1, shift)
		}
	case sdl.K_RIGHT:
		switch {
		case lo != hi && !shift:
			b.MoveTo(hi, false)
		case ctrl:
			b.MoveTo(b.WordRight(b.Cur), shift)
		default:
			b.MoveTo(b.Cur+1, shift)
		}
	case sdl.K_UP:
		e.vmove(-1, shift)
	case sdl.K_DOWN:
		e.vmove(1, shift)
	case sdl.K_PAGEUP:
		e.vmove(-max(1, e.visRows-1), shift)
	case sdl.K_PAGEDOWN:
		e.vmove(max(1, e.visRows-1), shift)
	case sdl.K_HOME:
		if ctrl {
			b.MoveTo(0, shift)
		} else {
			b.MoveTo(b.LineStart(b.Cur), shift)
		}
	case sdl.K_END:
		if ctrl {
			b.MoveTo(b.Len(), shift)
		} else {
			b.MoveTo(b.LineEnd(b.Cur), shift)
		}
	case sdl.K_BACKSPACE:
		b.Backspace(ctrl)
	case sdl.K_DELETE:
		b.Delete(ctrl)
	case sdl.K_RETURN, sdl.K_KP_ENTER:
		if !ctrl {
			b.Insert("\n")
		}
	case sdl.K_TAB:
		b.Insert("    ")
	}
}

// editText inserts typed text (keyboard layouts and input methods arrive here, not as keys).
func (a *app) editText(s string) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if s == "" { // SDL sends no text for Ctrl shortcuts
		return
	}
	e := a.ed
	e.buf.Insert(s)
	e.goal, e.warned, e.follow, e.lastAct = -1, false, true, time.Now()
}

func (e *editor) vmove(d int, extend bool) {
	rows := e.layout()
	r := edit.RowOf(rows, e.buf.Cur)
	if e.goal < 0 {
		e.goal = e.buf.Cur - rows[r].Start
	}
	nr := clamp(r+d, 0, len(rows)-1)
	p := edit.AtCol(rows, nr, e.goal)
	if nr == r { // already on the first or last row: go to that end
		p = 0
		if d > 0 {
			p = e.buf.Len()
		}
	}
	e.buf.MoveTo(p, extend)
	e.follow = true
}

// posAt maps a frame position to a text offset.
func (e *editor) posAt(x, y int) int {
	rows := e.layout()
	r := clamp(e.scroll+floorDiv(y-e.y0, max(e.rowH, 1)), 0, len(rows)-1)
	col := floorDiv(x-e.x0-2+e.adv/2, max(e.adv, 1))
	return edit.AtCol(rows, r, col)
}

func floorDiv(a, b int) int {
	if a < 0 {
		return -((-a + b - 1) / b)
	}
	return a / b
}

func (a *app) editClick(clicks int) {
	e := a.ed
	x, y := a.frameMouse(a.tw)
	p := e.posAt(int(x), int(y))
	switch {
	case clicks >= 3:
		e.buf.MoveTo(e.buf.LineStart(p), false)
		e.buf.MoveTo(e.buf.LineEnd(p), true)
	case clicks == 2:
		lo, hi := e.buf.WordAt(p)
		e.buf.MoveTo(lo, false)
		e.buf.MoveTo(hi, true)
	default:
		e.buf.MoveTo(p, sdl.GetModState()&sdl.KMOD_SHIFT != 0)
	}
	e.goal, e.warned, e.lastAct = -1, false, time.Now()
}

func (a *app) editDrag() {
	e := a.ed
	x, y := a.frameMouse(a.tw)
	e.buf.MoveTo(e.posAt(int(x), int(y)), true)
	e.follow, e.goal, e.lastAct = true, -1, time.Now()
}

var colSel = sdl.Color{R: 43, G: 70, B: 130, A: 255}

func (a *app) drawEditor(w, h int32) {
	e, b := a.ed, a.ed.buf
	a.r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	a.r.SetDrawColor(colBG.R, colBG.G, colBG.B, 255)
	a.r.FillRect(&sdl.Rect{W: w, H: h})

	// Header: title and buttons.
	hb := clamp(int(h)/14, 44, 96)
	bpx := hb * 9 / 20
	a.r.SetDrawColor(colRow.R, colRow.G, colRow.B, 255)
	a.r.FillRect(&sdl.Rect{W: w, H: int32(hb)})
	name := "new script"
	if a.path != "" {
		name = filepath.Base(a.path)
	}
	tk := faceKey{px: bpx}
	a.gl.draw(tk, "Editing "+name, 24, (hb-a.gl.lineHeight(tk))/2, colUpcoming)
	if b.Modified() {
		a.gl.draw(faceKey{px: bpx, italic: true}, "unsaved", 24+a.gl.measure(tk, "Editing "+name)+bpx, (hb-a.gl.lineHeight(tk))/2, colAccent)
	}
	btns := []rbtn{
		{"Save", func() { a.saveEditor() }},
		{"Save & close", func() {
			if a.saveEditor() {
				a.closeEditor()
			}
		}},
		{"Close", func() {
			if e.confirmDiscard("Close again discards them") {
				a.closeEditor()
			}
		}},
	}
	bw, gap := int32(bpx*7), int32(10)
	bx := w - 56 - int32(len(btns))*(bw+gap) // leave room for the listening dot
	for i, bt := range btns {
		a.button(sdl.Rect{X: bx + int32(i)*(bw+gap), Y: 8, W: bw, H: int32(hb - 16)}, bt.label, bpx, false, bt.fn)
		if a.ed == nil {
			return // a button closed the editor
		}
	}

	// Text area.
	k := faceKey{px: a.cfg.EditSize, mono: true}
	e.adv = max(1, a.gl.measure(k, "M"))
	e.rowH = a.gl.lineHeight(k) + a.cfg.EditSize/5
	e.x0 = max(24, int(w)/24)
	e.y0 = hb + 16
	small := faceKey{px: max(14, bpx*3/4), italic: true}
	footH := a.gl.lineHeight(small) + 16
	e.cols = max(10, (int(w)-2*e.x0)/e.adv)
	e.visRows = max(1, (int(h)-e.y0-footH)/e.rowH)
	rows := e.layout()
	cr := edit.RowOf(rows, b.Cur)
	if e.follow {
		if cr < e.scroll {
			e.scroll = cr
		} else if cr >= e.scroll+e.visRows {
			e.scroll = cr - e.visRows + 1
		}
		e.follow = false
	}
	e.scroll = clamp(e.scroll, 0, max(0, len(rows)-e.visRows))

	text := b.Runes()
	lo, hi := b.Selection()
	rr := -1
	if e.reading >= 0 && !b.Modified() {
		rr = edit.RowOf(rows, e.reading)
	}
	for r := e.scroll; r < min(len(rows), e.scroll+e.visRows); r++ {
		row := rows[r]
		y := e.y0 + (r-e.scroll)*e.rowH
		end := row.End
		if end < len(text) && text[end] == '\n' {
			end++ // a selected newline shows as one cell
		}
		if s, t := max(lo, row.Start), min(hi, end); s < t {
			a.r.SetDrawColor(colSel.R, colSel.G, colSel.B, 255)
			a.r.FillRect(&sdl.Rect{X: int32(e.x0 + 2 + (s-row.Start)*e.adv), Y: int32(y), W: int32((t - s) * e.adv), H: int32(e.rowH)})
		}
		if r == rr { // where the prompter was reading when the editor opened
			a.r.SetDrawColor(colAccent.R, colAccent.G, colAccent.B, 255)
			a.r.FillRect(&sdl.Rect{X: int32(e.x0 - 14), Y: int32(y + e.rowH/4), W: 6, H: int32(e.rowH / 2)})
		}
		line := strings.ReplaceAll(string(text[row.Start:row.End]), "\t", " ")
		a.gl.draw(k, line, e.x0, y+(e.rowH-a.gl.lineHeight(k))/2, colUpcoming)
	}
	if cr >= e.scroll && cr < e.scroll+e.visRows && time.Since(e.lastAct)%(1060*time.Millisecond) < 530*time.Millisecond {
		a.r.SetDrawColor(colNow.R, colNow.G, colNow.B, 255)
		a.r.FillRect(&sdl.Rect{X: int32(e.x0 + 2 + (b.Cur-rows[cr].Start)*e.adv - 1), Y: int32(e.y0 + (cr-e.scroll)*e.rowH), W: 3, H: int32(e.rowH)})
	}
	if len(rows) > e.visRows { // scroll position
		th := max(20, int(h-int32(e.y0)-int32(footH))*e.visRows/len(rows))
		ty := e.y0 + (int(h)-e.y0-footH-th)*e.scroll/max(1, len(rows)-e.visRows)
		a.r.SetDrawColor(colBtn.R, colBtn.G, colBtn.B, 255)
		a.r.FillRect(&sdl.Rect{X: w - 10, Y: int32(ty), W: 6, H: int32(th)})
	}

	msg := "Ctrl+S save  |  Ctrl+Enter save & close  |  Esc close  |  Ctrl+Z undo, Ctrl+Shift+Z redo  |  Ctrl+ +/- text size"
	if e.msg != "" && time.Since(e.msgAt) < 5*time.Second {
		msg = e.msg
	}
	a.gl.draw(small, msg, e.x0, int(h)-footH+6, colDone)
}
