// Package edit is the text buffer behind the in-app script editor: a cursor with an
// optional selection, undo/redo, and soft wrapping into rows of a fixed column count.
// Positions are rune offsets. It has no UI code, so it can be tested on its own.
package edit

import (
	"strings"
	"unicode"
)

// Buffer holds the text being edited.
type Buffer struct {
	text        []rune
	Cur, Anchor int // cursor and selection anchor (equal when nothing is selected)
	Version     int // bumped on every change, so callers can cache layout

	saved      string
	undo, redo []snap
	last       kind // what the previous edit was, for grouping undo steps
}

type snap struct {
	text        []rune
	cur, anchor int
}

type kind int

const (
	kindNone kind = iota
	kindType
	kindDelete
	kindOther
)

const maxUndo = 500

// New returns a buffer holding s, marked as saved.
func New(s string) *Buffer {
	return &Buffer{text: []rune(s), saved: s}
}

func (b *Buffer) String() string { return string(b.text) }
func (b *Buffer) Len() int       { return len(b.text) }
func (b *Buffer) Runes() []rune  { return b.text }

// Modified reports whether the text differs from the last saved version.
func (b *Buffer) Modified() bool { return string(b.text) != b.saved }

// MarkSaved records the current text as saved.
func (b *Buffer) MarkSaved() { b.saved = string(b.text); b.last = kindNone }

// Selection returns the selected range [lo, hi); lo == hi when nothing is selected.
func (b *Buffer) Selection() (lo, hi int) { return min(b.Cur, b.Anchor), max(b.Cur, b.Anchor) }

func (b *Buffer) Selected() string {
	lo, hi := b.Selection()
	return string(b.text[lo:hi])
}

func (b *Buffer) SelectAll() { b.Anchor, b.Cur, b.last = 0, len(b.text), kindNone }

// MoveTo places the cursor at p; extend keeps the anchor so the selection grows.
func (b *Buffer) MoveTo(p int, extend bool) {
	b.Cur = max(0, min(len(b.text), p))
	if !extend {
		b.Anchor = b.Cur
	}
	b.last = kindNone
}

// checkpoint saves an undo step unless this edit continues the previous one of the same kind.
func (b *Buffer) checkpoint(k kind) {
	if k == b.last && k != kindOther {
		return
	}
	b.undo = append(b.undo, snap{append([]rune(nil), b.text...), b.Cur, b.Anchor})
	if len(b.undo) > maxUndo {
		b.undo = b.undo[1:]
	}
	b.redo = nil
	b.last = k
}

func (b *Buffer) replace(lo, hi int, s []rune) {
	b.text = append(b.text[:lo:lo], append(s, b.text[hi:]...)...)
	b.Cur = lo + len(s)
	b.Anchor = b.Cur
	b.Version++
}

// Insert replaces the selection (if any) with s. Typing a word stays one undo step;
// a space or newline starts the next one.
func (b *Buffer) Insert(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	rs := []rune(s)
	lo, hi := b.Selection()
	if len(rs) == 0 && lo == hi {
		return
	}
	k := kindType
	if len(rs) != 1 || lo != hi {
		k = kindOther
	}
	b.checkpoint(k)
	b.replace(lo, hi, rs)
	if len(rs) == 1 && unicode.IsSpace(rs[0]) {
		b.last = kindNone
	}
}

// DeleteSelection removes the selected text and reports whether there was any.
func (b *Buffer) DeleteSelection() bool {
	lo, hi := b.Selection()
	if lo == hi {
		return false
	}
	b.checkpoint(kindOther)
	b.replace(lo, hi, nil)
	return true
}

// Backspace deletes the selection, or the rune (word) before the cursor.
func (b *Buffer) Backspace(word bool) {
	if b.DeleteSelection() || b.Cur == 0 {
		return
	}
	from := b.Cur - 1
	if word {
		from = b.WordLeft(b.Cur)
	}
	b.checkpoint(kindDelete)
	b.replace(from, b.Cur, nil)
}

// Delete deletes the selection, or the rune (word) after the cursor.
func (b *Buffer) Delete(word bool) {
	if b.DeleteSelection() || b.Cur == len(b.text) {
		return
	}
	to := b.Cur + 1
	if word {
		to = b.WordRight(b.Cur)
	}
	b.checkpoint(kindDelete)
	b.replace(b.Cur, to, nil)
}

func (b *Buffer) Undo() bool { return b.swap(&b.undo, &b.redo) }
func (b *Buffer) Redo() bool { return b.swap(&b.redo, &b.undo) }

func (b *Buffer) swap(from, to *[]snap) bool {
	if len(*from) == 0 {
		return false
	}
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, snap{b.text, b.Cur, b.Anchor})
	b.text, b.Cur, b.Anchor = s.text, s.cur, s.anchor
	b.Version++
	b.last = kindNone
	return true
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\'' || r == '’'
}

// WordLeft returns the start of the word before p.
func (b *Buffer) WordLeft(p int) int {
	for p > 0 && !isWord(b.text[p-1]) {
		p--
	}
	for p > 0 && isWord(b.text[p-1]) {
		p--
	}
	return p
}

// WordRight returns the end of the word after p.
func (b *Buffer) WordRight(p int) int {
	n := len(b.text)
	for p < n && !isWord(b.text[p]) {
		p++
	}
	for p < n && isWord(b.text[p]) {
		p++
	}
	return p
}

// WordAt returns the word around p (empty at p when p is not in or next to a word).
func (b *Buffer) WordAt(p int) (lo, hi int) {
	lo, hi = p, p
	for lo > 0 && isWord(b.text[lo-1]) {
		lo--
	}
	for hi < len(b.text) && isWord(b.text[hi]) {
		hi++
	}
	return lo, hi
}

// LineStart and LineEnd bound the logical (newline-separated) line containing p.
func (b *Buffer) LineStart(p int) int {
	for p > 0 && b.text[p-1] != '\n' {
		p--
	}
	return p
}

func (b *Buffer) LineEnd(p int) int {
	for p < len(b.text) && b.text[p] != '\n' {
		p++
	}
	return p
}

// Row is one displayed line: text[Start:End], without its trailing newline.
type Row struct{ Start, End int }

// Wrap breaks text into rows of at most cols runes, at spaces where possible.
// A row that ends in a soft wrap keeps its trailing space, so every offset
// belongs to exactly one row (see RowOf).
func Wrap(text []rune, cols int) []Row {
	cols = max(cols, 1)
	var rows []Row
	start := 0
	for {
		end := start
		for end < len(text) && text[end] != '\n' {
			end++
		}
		s := start
		for end-s > cols {
			brk := s + cols
			for i := brk; i > s; i-- {
				if text[i-1] == ' ' || text[i-1] == '\t' {
					brk = i
					break
				}
			}
			rows = append(rows, Row{s, brk})
			s = brk
		}
		rows = append(rows, Row{s, end})
		if end >= len(text) {
			return rows
		}
		start = end + 1
	}
}

// RowOf returns the index of the row the cursor at p is drawn on. At a soft wrap the
// cursor belongs to the start of the next row.
func RowOf(rows []Row, p int) int {
	lo, hi := 0, len(rows)-1
	for lo < hi { // last row with Start <= p
		mid := (lo + hi + 1) / 2
		if rows[mid].Start <= p {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// AtCol returns the offset in row r closest to column col.
func AtCol(rows []Row, r, col int) int {
	row := rows[r]
	end := row.End
	if r+1 < len(rows) && rows[r+1].Start == row.End && end > row.Start {
		end-- // soft-wrapped row: its last offset is drawn on the next row
	}
	return row.Start + max(0, min(col, end-row.Start))
}
