package edit

import (
	"reflect"
	"testing"
)

func TestTypingAndUndoGroups(t *testing.T) {
	b := New("")
	for _, r := range "hello world" {
		b.Insert(string(r))
	}
	if b.String() != "hello world" || !b.Modified() {
		t.Fatal(b.String())
	}
	b.Undo() // removes "world"
	if b.String() != "hello " {
		t.Fatalf("%q", b.String())
	}
	b.Undo()
	b.Undo()
	if b.String() != "" || b.Modified() {
		t.Fatalf("%q", b.String())
	}
	b.Redo()
	b.Redo()
	b.Redo()
	if b.String() != "hello world" || b.Cur != 11 {
		t.Fatalf("%q %d", b.String(), b.Cur)
	}
}

func TestSelectionReplaceAndDelete(t *testing.T) {
	b := New("one two three")
	b.MoveTo(4, false)
	b.MoveTo(7, true)
	if b.Selected() != "two" {
		t.Fatal(b.Selected())
	}
	b.Insert("2")
	if b.String() != "one 2 three" || b.Cur != 5 {
		t.Fatalf("%q %d", b.String(), b.Cur)
	}
	b.MoveTo(b.Len(), false)
	b.Backspace(true)
	if b.String() != "one 2 " {
		t.Fatalf("%q", b.String())
	}
	b.MoveTo(0, false)
	b.Delete(true)
	if b.String() != " 2 " {
		t.Fatalf("%q", b.String())
	}
	b.MarkSaved()
	if b.Modified() {
		t.Fatal("still modified after save")
	}
}

func TestInsertNormalizesCRLF(t *testing.T) {
	b := New("")
	b.Insert("a\r\nb")
	if b.String() != "a\nb" {
		t.Fatalf("%q", b.String())
	}
}

func TestWrap(t *testing.T) {
	text := []rune("the quick brown fox\n\nsupercalifragilistic")
	got := Wrap(text, 10)
	want := []Row{{0, 10}, {10, 19}, {20, 20}, {21, 31}, {31, 41}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	for p, r := range map[int]int{0: 0, 9: 0, 10: 1, 19: 1, 20: 2, 21: 3, 31: 4, 41: 4} {
		if g := RowOf(got, p); g != r {
			t.Errorf("RowOf(%d) = %d, want %d", p, g, r)
		}
	}
	if p := AtCol(got, 0, 99); p != 9 { // soft-wrapped row stops before its last offset
		t.Errorf("AtCol soft = %d", p)
	}
	if p := AtCol(got, 1, 99); p != 19 { // hard line end
		t.Errorf("AtCol hard = %d", p)
	}
}

func TestWrapTrailingNewline(t *testing.T) {
	got := Wrap([]rune("ab\n"), 10)
	if want := []Row{{0, 2}, {3, 3}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if RowOf(got, 3) != 1 {
		t.Fatal("cursor after final newline should be on the empty last row")
	}
}
