// Package asr captures a microphone natively and runs offline Vosk recognition on it.
package asr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"

	"github.com/CameronWeller/ai-speech-scroller/internal/vosk"
)

//go:embed model.tar.gz
var modelArchive []byte

const modelName = "vosk-model-small-en-us-0.15"
const sampleRate = 16000

// ModelDir extracts the embedded model to the user cache once and returns its path.
func ModelDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(cache, "ai-speech-scroller")
	dir := filepath.Join(root, modelName)
	if _, err := os.Stat(filepath.Join(dir, "am", "final.mdl")); err == nil {
		return dir, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(modelArchive))
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		dst := filepath.Join(root, filepath.Clean(h.Name))
		if !strings.HasPrefix(dst, root+string(filepath.Separator)) {
			return "", errors.New("unsafe path in model archive: " + h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(dst, 0o755)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
				err = writeFile(dst, tr)
			}
		}
		if err != nil {
			return "", err
		}
	}
	return dir, nil
}

func writeFile(path string, r io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

type Mic struct {
	Name string
	id   malgo.DeviceID
}

// Engine owns the audio context and the loaded model.
type Engine struct {
	ctx   *malgo.AllocatedContext
	model *vosk.Model
	mics  []Mic

	mu      sync.Mutex
	dev     *malgo.Device
	stopped chan struct{}
	chunks  chan []byte
}

func New() (*Engine, error) {
	dir, err := ModelDir()
	if err != nil {
		return nil, err
	}
	model, err := vosk.NewModel(dir)
	if err != nil {
		return nil, err
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return nil, err
	}
	e := &Engine{ctx: ctx, model: model}
	e.RefreshMics()
	return e, nil
}

// RefreshMics re-reads the capture device list.
func (e *Engine) RefreshMics() []Mic {
	infos, err := e.ctx.Devices(malgo.Capture)
	e.mics = nil
	if err == nil {
		for _, d := range infos {
			if strings.HasPrefix(d.Name(), "Monitor of ") {
				continue // PulseAudio loopback of an output, not a microphone
			}
			e.mics = append(e.mics, Mic{Name: d.Name(), id: d.ID})
		}
	}
	return e.mics
}

func (e *Engine) Mics() []Mic { return e.mics }

// Running reports whether a microphone is being listened to.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dev != nil
}

// Start listens to mic i and calls onWords for every partial/final hypothesis.
func (e *Engine) Start(mic int, onWords func(words []string, final bool)) error {
	e.Stop()
	rec, err := vosk.NewRecognizer(e.model, sampleRate)
	if err != nil {
		return err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = sampleRate
	cfg.PeriodSizeInMilliseconds = 100
	if mic >= 0 && mic < len(e.mics) {
		cfg.Capture.DeviceID = e.mics[mic].id.Pointer()
	}
	chunks := make(chan []byte, 64)
	dev, err := malgo.InitDevice(e.ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			c := append([]byte(nil), in...)
			select {
			case chunks <- c:
			default: // recogniser behind: drop rather than block the audio thread
			}
		},
	})
	if err != nil {
		rec.Close()
		return fmt.Errorf("open microphone: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		rec.Close()
		return fmt.Errorf("start microphone: %w", err)
	}
	stopped := make(chan struct{})
	e.mu.Lock()
	e.dev, e.stopped = dev, stopped
	e.mu.Unlock()
	go func() {
		defer close(stopped)
		defer rec.Close()
		for c := range chunks {
			var raw string
			final := rec.Accept(c)
			if final {
				raw = rec.Result()
			} else {
				raw = rec.Partial()
			}
			var msg struct {
				Text    string `json:"text"`
				Partial string `json:"partial"`
			}
			if json.Unmarshal([]byte(raw), &msg) != nil {
				continue
			}
			text := msg.Text + msg.Partial
			if w := strings.Fields(text); len(w) > 0 {
				onWords(w, final)
			}
		}
	}()
	e.chunks = chunks
	return nil
}

// Stop releases the microphone.
func (e *Engine) Stop() {
	e.mu.Lock()
	dev, stopped, chunks := e.dev, e.stopped, e.chunks
	e.dev, e.stopped, e.chunks = nil, nil, nil
	e.mu.Unlock()
	if dev == nil {
		return
	}
	dev.Uninit() // no more callbacks after this returns
	close(chunks)
	<-stopped
}
