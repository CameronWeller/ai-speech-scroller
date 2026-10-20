package main

import (
	"fmt"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

// hit is a clickable rectangle registered while a frame is drawn (in unmirrored frame coordinates).
type hit struct {
	r  sdl.Rect
	fn func()
}

var (
	colBtn   = sdl.Color{R: 38, G: 44, B: 52, A: 255}
	colHover = sdl.Color{R: 66, G: 78, B: 94, A: 255}
	colOn    = sdl.Color{R: 43, G: 108, B: 255, A: 255}
	colRow   = sdl.Color{R: 22, G: 26, B: 32, A: 255}
)

func (a *app) mouseActive() bool { return time.Since(a.lastMove) < 3*time.Second }

// frameMouse converts the window mouse position to frame coordinates (undoing the mirror flip).
func (a *app) frameMouse(w int32) (int32, int32) {
	if a.cfg.Mirror {
		return w - a.mx, a.my
	}
	return a.mx, a.my
}

func (a *app) click() {
	x, y := a.frameMouse(a.tw)
	for i := len(a.hits) - 1; i >= 0; i-- {
		h := a.hits[i]
		if x >= h.r.X && x < h.r.X+h.r.W && y >= h.r.Y && y < h.r.Y+h.r.H {
			h.fn()
			return
		}
	}
}

// button draws a labelled button, registers its click, and returns nothing.
func (a *app) button(r sdl.Rect, label string, px int, on bool, fn func()) {
	mx, my := a.frameMouse(a.tw)
	hover := mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H
	c := colBtn
	if on {
		c = colOn
	}
	if hover {
		c = colHover
		if on {
			c = sdl.Color{R: 80, G: 135, B: 255, A: 255}
		}
	}
	a.r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	a.r.SetDrawColor(c.R, c.G, c.B, 255)
	a.r.FillRect(&r)
	k := faceKey{px: px}
	tw := a.gl.measure(k, label)
	a.gl.draw(k, label, int(r.X)+(int(r.W)-tw)/2, int(r.Y)+(int(r.H)-a.gl.lineHeight(k))/2, colUpcoming)
	a.hits = append(a.hits, hit{r, fn})
}

// ---- menu ----

type rbtn struct {
	label string
	fn    func()
}

type row struct {
	label    string
	value    func() string
	dec, inc func() // adjustable rows: -/+ (or previous/next)
	toggle   func() // on/off rows
	btns     []rbtn // action rows
}

func onoff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

func (a *app) rows() []row {
	micName := func() string {
		if a.micIdx < 0 {
			return "System default"
		}
		return a.eng.Mics()[a.micIdx].Name
	}
	nDisp := func() int { n, _ := sdl.GetNumVideoDisplays(); return n }
	return []row{
		{label: "Listening", value: func() string { return onoff(a.listening) }, toggle: a.toggleListening},
		{label: "Microphone", value: micName, dec: func() { a.cycleMic(-1) }, inc: func() { a.cycleMic(1) }},
		{label: "Screen", value: func() string { return displayName(a.dispIdx) },
			dec: func() { a.moveToDisplay((a.dispIdx + nDisp() - 1) % nDisp()) },
			inc: func() { a.moveToDisplay((a.dispIdx + 1) % nDisp()) }},
		{label: "Text size", value: func() string { return fmt.Sprint(a.cfg.Size) },
			dec: func() { a.cfg.Size = clamp(a.cfg.Size-4, 24, 240) }, inc: func() { a.cfg.Size = clamp(a.cfg.Size+4, 24, 240) }},
		{label: "Column width", value: func() string { return fmt.Sprintf("%d%%", a.cfg.Column) },
			dec: func() { a.cfg.Column = clamp(a.cfg.Column-5, 30, 98) }, inc: func() { a.cfg.Column = clamp(a.cfg.Column+5, 30, 98) }},
		{label: "Reading line", value: func() string { return fmt.Sprintf("%d%%", a.cfg.Guide) },
			dec: func() { a.cfg.Guide = clamp(a.cfg.Guide-3, 10, 70) }, inc: func() { a.cfg.Guide = clamp(a.cfg.Guide+3, 10, 70) }},
		{label: "Mirror", value: func() string { return onoff(a.cfg.Mirror) }, toggle: func() { a.cfg.Mirror = !a.cfg.Mirror }},
		{label: "Position", btns: []rbtn{{"Restart", func() { a.nudge(-1 << 30) }}, {"Back 5", func() { a.nudge(-5) }}, {"Forward 5", func() { a.nudge(5) }}}},
		{label: "", btns: []rbtn{{"Close menu", func() { a.menuOpen = false; a.cfg.save() }}, {"Quit", func() { a.quit = true }}}},
	}
}

func (a *app) menuKey(k sdl.Keycode) {
	rows := a.rows()
	r := rows[a.menuSel]
	switch k {
	case sdl.K_UP:
		a.menuSel, a.menuSub = (a.menuSel+len(rows)-1)%len(rows), 0
	case sdl.K_DOWN:
		a.menuSel, a.menuSub = (a.menuSel+1)%len(rows), 0
	case sdl.K_LEFT:
		if r.dec != nil {
			r.dec()
		} else if r.btns != nil {
			a.menuSub = (a.menuSub + len(r.btns) - 1) % len(r.btns)
		}
	case sdl.K_RIGHT:
		if r.inc != nil {
			r.inc()
		} else if r.btns != nil {
			a.menuSub = (a.menuSub + 1) % len(r.btns)
		}
	case sdl.K_RETURN, sdl.K_SPACE, sdl.K_KP_ENTER:
		switch {
		case r.toggle != nil:
			r.toggle()
		case r.btns != nil:
			r.btns[min(a.menuSub, len(r.btns)-1)].fn()
		case r.inc != nil:
			r.inc()
		}
	}
}

func (a *app) drawMenu(w, h int32, heard, status string) {
	a.r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	a.r.SetDrawColor(0, 0, 0, 232)
	a.r.FillRect(&sdl.Rect{W: w, H: h})
	rows := a.rows()
	rh := (int(h) - 24) / (len(rows) + 1)
	rh = min(rh, 72)
	px := rh * 11 / 20
	pw := min(int(w)-32, 1000)
	x0 := (int(w) - pw) / 2
	y := (int(h) - rh*(len(rows)+1)) / 2
	cx, cw := x0+pw*38/100, pw*62/100-8 // control area
	mx, my := a.frameMouse(w)
	for i, r := range rows {
		rect := sdl.Rect{X: int32(x0), Y: int32(y), W: int32(pw), H: int32(rh - 4)}
		if i == a.menuSel || (my >= rect.Y && my < rect.Y+rect.H && mx >= rect.X && mx < rect.X+rect.W && a.mouseActive()) {
			if my >= rect.Y && my < rect.Y+rect.H && a.mouseActive() {
				a.menuSel = i
			}
			a.r.SetDrawColor(colRow.R, colRow.G, colRow.B, 255)
			a.r.FillRect(&rect)
			a.r.SetDrawColor(colAccent.R, colAccent.G, colAccent.B, 255)
			a.r.FillRect(&sdl.Rect{X: rect.X, Y: rect.Y, W: 5, H: rect.H})
		}
		k := faceKey{px: px}
		ly := y + (rh-4-a.gl.lineHeight(k))/2
		a.gl.draw(k, r.label, x0+18, ly, colUpcoming)
		bh := int32(rh - 12)
		by := int32(y + 4)
		switch {
		case r.dec != nil:
			bw := bh * 3 / 2
			a.button(sdl.Rect{X: int32(cx), Y: by, W: bw, H: bh}, "<", px, false, r.dec)
			a.button(sdl.Rect{X: int32(cx+cw) - bw, Y: by, W: bw, H: bh}, ">", px, false, r.inc)
			v := r.value()
			vx := cx + int(bw) + (cw-2*int(bw))/2 - a.gl.measure(k, v)/2
			a.gl.draw(k, v, vx, ly, colAccent)
		case r.toggle != nil:
			a.button(sdl.Rect{X: int32(cx), Y: by, W: int32(cw), H: bh}, r.value(), px, r.value() == "On", r.toggle)
		case r.btns != nil:
			gap := 10
			bw := (cw - gap*(len(r.btns)-1)) / len(r.btns)
			for j, b := range r.btns {
				rc := sdl.Rect{X: int32(cx + j*(bw+gap)), Y: by, W: int32(bw), H: bh}
				a.button(rc, b.label, px, i == a.menuSel && j == a.menuSub, func(fn func()) func() { return fn }(b.fn))
			}
		}
		y += rh
	}
	small := faceKey{px: max(14, px*2/3), italic: true}
	msg := "Esc closes  |  mouse, or arrows + Enter  |  heard: " + heard
	if status != "" {
		msg = status
	}
	a.gl.draw(small, msg, x0+8, y+2, colDone)
}

// ---- transport bar (shown while the mouse is moving) ----

func (a *app) drawBar(w, h int32) {
	bh := h / 9
	px := int(bh) * 11 / 20
	y := h - bh - 14
	type b struct {
		label string
		on    bool
		fn    func()
	}
	items := []b{
		{"Listen " + onoff(a.listening), a.listening, a.toggleListening},
		{"Restart", false, func() { a.nudge(-1 << 30) }},
		{"< 5", false, func() { a.nudge(-5) }},
		{"5 >", false, func() { a.nudge(5) }},
		{"A -", false, func() { a.cfg.Size = clamp(a.cfg.Size-4, 24, 240) }},
		{"A +", false, func() { a.cfg.Size = clamp(a.cfg.Size+4, 24, 240) }},
		{"Mirror", a.cfg.Mirror, func() { a.cfg.Mirror = !a.cfg.Mirror }},
		{"Menu", false, func() { a.menuOpen = true }},
	}
	gap := int32(8)
	bw := (w - 28 - gap*int32(len(items)-1)) / int32(len(items))
	a.r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	a.r.SetDrawColor(0, 0, 0, 200)
	a.r.FillRect(&sdl.Rect{X: 0, Y: y - 10, W: w, H: bh + 24})
	for i, it := range items {
		a.button(sdl.Rect{X: 14 + int32(i)*(bw+gap), Y: y, W: bw, H: bh}, it.label, px, it.on, it.fn)
	}
}

// dot draws a small status square: green while listening, grey when paused.
func (a *app) dot(w int32) {
	c := sdl.Color{R: 90, G: 100, B: 112, A: 255}
	if a.listening {
		c = sdl.Color{R: 70, G: 200, B: 110, A: 255}
	}
	a.r.SetDrawColor(c.R, c.G, c.B, 255)
	a.r.FillRect(&sdl.Rect{X: w - 36, Y: 20, W: 14, H: 14})
}

// drawCursor paints a large high-contrast arrow at the real mouse position (after any mirror flip).
func (a *app) drawCursor(h int32) {
	s := max(24, int(h)/16)
	fill := func(ox, oy, size int, c sdl.Color) {
		a.r.SetDrawColor(c.R, c.G, c.B, 255)
		for dy := 0; dy <= size*14/10; dy++ {
			var x1 int
			if dy <= size {
				x1 = dy * 72 / 100
			} else {
				x1 = size*72/100 - (dy-size)*2
			}
			if x1 > 0 {
				a.r.FillRect(&sdl.Rect{X: int32(ox), Y: int32(oy + dy), W: int32(x1), H: 1})
			}
		}
	}
	fill(int(a.mx)-3, int(a.my)-3, s+6, sdl.Color{A: 255})
	fill(int(a.mx), int(a.my), s, sdl.Color{R: 255, G: 255, B: 255, A: 255})
}
