// Package vosk is a small cgo wrapper over libvosk (Apache-2.0), enough for streaming recognition.
package vosk

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR}/../../lib -lvosk -Wl,-rpath,$ORIGIN -Wl,-rpath,$ORIGIN/lib -Wl,-rpath,/usr/lib/ai-speech-scroller
#include <stdlib.h>
#include "vosk_api.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

func init() { C.vosk_set_log_level(-1) }

type Model struct{ m *C.VoskModel }

func NewModel(dir string) (*Model, error) {
	c := C.CString(dir)
	defer C.free(unsafe.Pointer(c))
	m := C.vosk_model_new(c)
	if m == nil {
		return nil, errors.New("vosk: cannot load model from " + dir)
	}
	return &Model{m}, nil
}

type Recognizer struct{ r *C.VoskRecognizer }

func NewRecognizer(m *Model, rate float32) (*Recognizer, error) {
	r := C.vosk_recognizer_new(m.m, C.float(rate))
	if r == nil {
		return nil, errors.New("vosk: cannot create recognizer")
	}
	return &Recognizer{r}, nil
}

// Accept feeds 16-bit little-endian mono PCM and reports whether an utterance just ended.
func (r *Recognizer) Accept(pcm []byte) bool {
	if len(pcm) == 0 {
		return false
	}
	return C.vosk_recognizer_accept_waveform(r.r, (*C.char)(unsafe.Pointer(&pcm[0])), C.int(len(pcm))) != 0
}

func (r *Recognizer) Result() string  { return C.GoString(C.vosk_recognizer_result(r.r)) }
func (r *Recognizer) Partial() string { return C.GoString(C.vosk_recognizer_partial_result(r.r)) }
func (r *Recognizer) Final() string   { return C.GoString(C.vosk_recognizer_final_result(r.r)) }
func (r *Recognizer) Close()          { C.vosk_recognizer_free(r.r) }
