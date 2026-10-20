package match

import (
	"reflect"
	"testing"
)

const script = "Good evening everyone. Tonight we are talking about the history of the telescope, " +
	"and how a simple lens changed astronomy forever. Galileo built his first instrument in 1609."

func f(s string) []string { return splitWords(s) }
func splitWords(s string) []string {
	var o []string
	cur := ""
	for _, r := range s + " " {
		if r == ' ' {
			if cur != "" {
				o = append(o, cur)
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	return o
}

func TestAdvancesOnExactWords(t *testing.T) {
	m := New(script)
	m.Feed(f("good evening everyone"))
	if m.Pos() != 3 {
		t.Fatal(m.Pos())
	}
}

func TestRepeatedPartialsIdempotent(t *testing.T) {
	m := New(script)
	m.Feed(f("good evening"))
	m.Feed(f("good evening"))
	m.Feed(f("good evening everyone"))
	if m.Pos() != 3 {
		t.Fatal(m.Pos())
	}
}

func TestSkipsMissedWords(t *testing.T) {
	m := New(script)
	m.Feed(f("good evening everyone"))
	m.Feed(f("talking about")) // "tonight we are" missed
	if m.words[m.Pos()-1] != "about" {
		t.Fatal(m.words[m.Pos()-1])
	}
}

func TestFuzzyMatch(t *testing.T) {
	m := New(script)
	m.Feed(f("good evening everyone tonight we are talking about the history of the"))
	m.Feed(f("telescopes"))
	if m.words[m.Pos()-1] != "telescope" {
		t.Fatal(m.words[m.Pos()-1])
	}
	if !WordsMatch("astron", "astronomy") || WordsMatch("as", "astronomy") {
		t.Fatal("prefix rule")
	}
}

func TestNeverBackward(t *testing.T) {
	m := New(script)
	m.Feed(f("good evening everyone tonight we are"))
	p := m.Pos()
	m.Feed(f("good evening"))
	if m.Pos() != p {
		t.Fatal("moved backward")
	}
}

func TestLoneStopwordAndFarJump(t *testing.T) {
	m := New(script)
	m.Feed(f("good"))
	m.Feed(f("the"))
	if m.Pos() != 1 {
		t.Fatal(m.Pos())
	}
	m.Feed(f("galileo")) // far ahead, no context
	if m.Pos() != 1 {
		t.Fatal(m.Pos())
	}
	m2 := New("one two three four five six seven eight nine ten eleven twelve alpha bravo charlie")
	m2.Feed(f("twelve alpha"))
	if m2.Pos() != 13 {
		t.Fatal(m2.Pos())
	}
}

func TestStageDirectionsNotSpoken(t *testing.T) {
	m := New("[A-cam] CLOSE\n\nAnd it costs less.\n\n→ GFX 9 · Price\n\nTry it now.")
	want := []string{"and", "it", "costs", "less", "try", "it", "now"}
	if !reflect.DeepEqual(m.words, want) {
		t.Fatal(m.words)
	}
	m.Feed(f("less try"))
	if m.words[m.Pos()-1] != "try" {
		t.Fatal("did not cross the cue")
	}
}
