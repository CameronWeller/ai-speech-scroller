// wavtest feeds a 16 kHz mono 16-bit WAV through Vosk and the matcher: wavtest script.txt speech.wav
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/CameronWeller/ai-speech-scroller/internal/asr"
	"github.com/CameronWeller/ai-speech-scroller/internal/match"
	"github.com/CameronWeller/ai-speech-scroller/internal/vosk"
)

func main() {
	script, _ := os.ReadFile(os.Args[1])
	wav, _ := os.ReadFile(os.Args[2])
	dir, err := asr.ModelDir()
	check(err)
	model, err := vosk.NewModel(dir)
	check(err)
	rec, err := vosk.NewRecognizer(model, 16000)
	check(err)
	m := match.New(string(script))
	pcm := wav[44:]
	for i := 0; i < len(pcm); i += 3200 {
		end := min(i+3200, len(pcm))
		var raw string
		if rec.Accept(pcm[i:end]) {
			raw = rec.Result()
		} else {
			raw = rec.Partial()
		}
		var msg struct{ Text, Partial string }
		_ = json.Unmarshal([]byte(raw), &msg)
		if w := strings.Fields(msg.Text + msg.Partial); len(w) > 0 {
			before := m.Pos()
			m.Feed(w)
			if m.Pos() != before {
				fmt.Printf("heard %-40q pos %d/%d\n", strings.Join(w, " "), m.Pos(), m.NumWords())
			}
		}
	}
	fmt.Println("final:", rec.Final(), "pos", m.Pos(), "of", m.NumWords())
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
