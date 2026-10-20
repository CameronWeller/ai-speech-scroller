package main

import (
	"image"
	"sync"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// glyphs renders strings to white-on-transparent SDL textures (tinted at draw time) and caches them.
type glyphs struct {
	r     *sdl.Renderer
	bold  *opentype.Font
	ital  *opentype.Font
	faces map[faceKey]font.Face
	tex   map[strKey]*glyph
	mu    sync.Mutex
}

type faceKey struct {
	px     int
	italic bool
}
type strKey struct {
	faceKey
	s string
}
type glyph struct {
	t    *sdl.Texture
	w, h int32
}

func newGlyphs(r *sdl.Renderer) *glyphs {
	b, err := opentype.Parse(gobold.TTF)
	if err != nil {
		panic(err)
	}
	i, err := opentype.Parse(goitalic.TTF)
	if err != nil {
		panic(err)
	}
	return &glyphs{r: r, bold: b, ital: i, faces: map[faceKey]font.Face{}, tex: map[strKey]*glyph{}}
}

func (g *glyphs) face(k faceKey) font.Face {
	if f, ok := g.faces[k]; ok {
		return f
	}
	src := g.bold
	if k.italic {
		src = g.ital
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: float64(k.px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	g.faces[k] = f
	return f
}

// measure returns the advance width of s in pixels.
func (g *glyphs) measure(k faceKey, s string) int {
	return font.MeasureString(g.face(k), s).Ceil()
}

func (g *glyphs) lineHeight(k faceKey) int {
	m := g.face(k).Metrics()
	return (m.Ascent + m.Descent).Ceil() + 4
}

// get returns the texture for s, rendering it on first use.
func (g *glyphs) get(k faceKey, s string) *glyph {
	sk := strKey{k, s}
	if t, ok := g.tex[sk]; ok {
		return t
	}
	if len(g.tex) > 6000 { // bound memory on very long scripts
		g.clear()
	}
	f := g.face(k)
	w, h := g.measure(k, s)+4, g.lineHeight(k)
	a := image.NewAlpha(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: a, Src: image.Opaque, Face: f, Dot: fixed.P(2, f.Metrics().Ascent.Ceil()+2)}
	d.DrawString(s)
	pix := make([]byte, w*h*4)
	for i, c := range a.Pix {
		pix[i*4], pix[i*4+1], pix[i*4+2], pix[i*4+3] = 255, 255, 255, c
	}
	t, err := g.r.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, int32(w), int32(h))
	if err != nil {
		return nil
	}
	_ = t.Update(nil, unsafe.Pointer(&pix[0]), w*4)
	_ = t.SetBlendMode(sdl.BLENDMODE_BLEND)
	gl := &glyph{t, int32(w), int32(h)}
	g.tex[sk] = gl
	return gl
}

func (g *glyphs) clear() {
	for _, t := range g.tex {
		t.t.Destroy()
	}
	g.tex = map[strKey]*glyph{}
}

// draw paints s with its top-left at (x,y), tinted with c.
func (g *glyphs) draw(k faceKey, s string, x, y int, c sdl.Color) {
	gl := g.get(k, s)
	if gl == nil {
		return
	}
	_ = gl.t.SetColorMod(c.R, c.G, c.B)
	_ = gl.t.SetAlphaMod(c.A)
	_ = g.r.Copy(gl.t, nil, &sdl.Rect{X: int32(x), Y: int32(y), W: gl.w, H: gl.h})
}
