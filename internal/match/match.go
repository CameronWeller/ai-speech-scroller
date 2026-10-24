// Package match follows a script through misrecognised speech.
//
// Feed takes the tail of the latest recognition hypothesis (partial or final).
// The position only moves forward, and feeding the same words twice is a no-op,
// so partial results that repeat or get revised are safe.
package match

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an and are as at be but by for from had has have he her his i if in is it its me my of on or our she so that the their them then there they this to was we were what when which who will with you your") {
		stopwords[w] = true
	}
}

// Token is one whitespace-separated piece of the script.
type Token struct {
	Raw    string
	Norm   string
	Spoken bool // has a normalized form and is not a stage direction
	Cue    bool // stage direction: [brackets] or a line starting with → / ->
	Br     int  // line breaks before this token (0 = same line)
	Off    int  // rune offset of Raw in the script (after \r\n -> \n)
}

// Normalize lowercases and strips everything but letters and digits.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r == '\'' || r == '’' {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var cueLine = regexp.MustCompile(`^\s*(→|->|\[)`)

// Tokenize splits script text into display tokens.
func Tokenize(text string) []Token {
	var out []Token
	br, lineOff := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		off := lineOff
		lineOff += utf8.RuneCountInString(line) + 1
		if strings.TrimSpace(line) == "" {
			br++
			continue
		}
		cl := cueLine.MatchString(line)
		inBracket, first := false, true
		for _, f := range fields(line) {
			raw := f.s
			if strings.Contains(raw, "[") {
				inBracket = true
			}
			cue := cl || inBracket
			norm := ""
			if !cue {
				norm = Normalize(raw)
			}
			t := Token{Raw: raw, Norm: norm, Spoken: norm != "", Cue: cue, Off: off + f.off}
			if first {
				t.Br = br + 1
			}
			out = append(out, t)
			first = false
			if strings.Contains(raw, "]") {
				inBracket = false
			}
		}
		br = 0
	}
	return out
}

type field struct {
	s   string
	off int // rune offset within the line
}

// fields is strings.Fields that also reports where each field starts.
func fields(line string) []field {
	var out []field
	start, startRune, n := -1, 0, 0
	for i, r := range line {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, field{line[start:i], startRune})
				start = -1
			}
		} else if start < 0 {
			start, startRune = i, n
		}
		n++
	}
	if start >= 0 {
		out = append(out, field{line[start:], startRune})
	}
	return out
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for i := range prev {
		prev[i] = i
	}
	for i := 1; i <= len(a); i++ {
		diag := prev[0]
		prev[0] = i
		for j := 1; j <= len(b); j++ {
			tmp := prev[j]
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			prev[j] = min(prev[j]+1, prev[j-1]+1, diag+cost)
			diag = tmp
		}
	}
	return prev[len(b)]
}

// WordsMatch reports whether a heard word is close enough to an expected one:
// exact, a prefix (partial results cut words short), or fuzzy (similarity >= 0.72).
func WordsMatch(heard, expected string) bool {
	if heard == expected {
		return true
	}
	h, e := []rune(heard), []rune(expected)
	n := max(len(h), len(e))
	if n < 4 {
		return false
	}
	if len(h) >= 4 && strings.HasPrefix(expected, heard) {
		return true
	}
	return 1-float64(levenshtein(h, e))/float64(n) >= 0.72
}

// Matcher tracks how far through the script the speaker is.
type Matcher struct {
	Tokens    []Token
	words     []string
	spokenIdx []int
	pos       int // next spoken word we expect to hear
	Lookahead int
	NearSkip  int
	Tail      int
}

// New builds a matcher for script text.
func New(text string) *Matcher {
	m := &Matcher{Tokens: Tokenize(text), Lookahead: 12, NearSkip: 3, Tail: 4}
	for i, t := range m.Tokens {
		if t.Spoken {
			m.spokenIdx = append(m.spokenIdx, i)
			m.words = append(m.words, t.Norm)
		}
	}
	return m
}

type cand struct{ i, dist, ctx, score int }

func (m *Matcher) candidates(heard []string, k int) []cand {
	w := heard[k]
	end := min(len(m.words), m.pos+m.Lookahead+1)
	var found []cand
	for i := m.pos; i < end; i++ {
		if !WordsMatch(w, m.words[i]) {
			continue
		}
		dist := i - m.pos
		stop := stopwords[m.words[i]]
		ctx := 0
		for b := 1; b <= k && b <= i; b++ {
			if !WordsMatch(heard[k-b], m.words[i-b]) {
				break
			}
			ctx++
		}
		if stop && dist > 0 && ctx == 0 {
			continue // "the" must not cause a jump
		}
		if dist > m.NearSkip && ctx == 0 {
			continue // far jumps need two words in a row
		}
		score := ctx*3 - dist
		if stop {
			score -= 2
		}
		found = append(found, cand{i, dist, ctx, score})
	}
	return found
}

// Feed advances the position using the heard words and returns it.
func (m *Matcher) Feed(heardRaw []string) int {
	var heard []string
	for _, w := range heardRaw {
		if n := Normalize(w); n != "" {
			heard = append(heard, n)
		}
	}
	if len(heard) > m.Tail {
		heard = heard[len(heard)-m.Tail:]
	}
	for k := range heard {
		found := m.candidates(heard, k)
		if len(found) == 0 {
			continue
		}
		best := found[0]
		for _, c := range found[1:] {
			if c.score > best.score || (c.score == best.score && c.dist < best.dist) {
				best = c
			}
		}
		m.pos = max(m.pos, best.i+1)
	}
	return m.pos
}

func (m *Matcher) Pos() int      { return m.pos }
func (m *Matcher) NumWords() int { return len(m.words) }

// TokenPos is the display-token index of the next word to be spoken.
func (m *Matcher) TokenPos() int {
	if m.pos < len(m.spokenIdx) {
		return m.spokenIdx[m.pos]
	}
	return len(m.Tokens)
}

// Reset moves to a spoken-word position (clamped).
func (m *Matcher) Reset(p int) { m.pos = max(0, min(len(m.words), p)) }
