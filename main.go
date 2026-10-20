// ai-speech-scroller: a teleprompter that follows your voice.
//
// One static binary: it serves the embedded UI, relays control messages to the
// display page over Server-Sent Events, and can open the display fullscreen on
// a chosen monitor. Speech recognition runs offline in the control page.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

//go:embed all:web
var webFS embed.FS

var version = "dev"

// hub fans control messages out to every connected display and remembers the
// latest message of each type so a display that reconnects catches up.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	latest  map[string][]byte
}

func newHub() *hub {
	return &hub{clients: map[chan []byte]struct{}{}, latest: map[string][]byte{}}
}

func (h *hub) publish(kind string, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest[kind] = msg
	for c := range h.clients {
		select {
		case c <- msg:
		default: // slow display: drop; the next pos message supersedes it
		}
	}
}

func (h *hub) subscribe() (chan []byte, [][]byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := make(chan []byte, 64)
	h.clients[c] = struct{}{}
	var replay [][]byte
	for _, k := range []string{"script", "style", "pos"} {
		if m, ok := h.latest[k]; ok {
			replay = append(replay, m)
		}
	}
	return c, replay
}

func (h *hub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *hub) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c, replay := h.subscribe()
	defer h.unsubscribe(c)
	for _, m := range replay {
		fmt.Fprintf(w, "data: %s\n\n", m)
	}
	fl.Flush()
	for {
		select {
		case m := <-c:
			fmt.Fprintf(w, "data: %s\n\n", m)
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (h *hub) handleMsg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &head) != nil || head.Type == "" {
		http.Error(w, "message needs a type", http.StatusBadRequest)
		return
	}
	h.publish(head.Type, body)
	w.WriteHeader(http.StatusNoContent)
}

var xrandrMon = regexp.MustCompile(`\d+/\d+x\d+/\d+\+(\d+)\+(\d+)\s+(\S+)$`)

// monitorOrigin finds a monitor's top-left corner by name using xrandr (Linux/X11 and XWayland).
func monitorOrigin(name string) (x, y int, err error) {
	out, err := exec.Command("xrandr", "--listmonitors").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("xrandr unavailable (%v); use -display-at X,Y", err)
	}
	var seen []string
	for _, line := range strings.Split(string(out), "\n") {
		m := xrandrMon.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		seen = append(seen, m[3])
		if m[3] == name {
			fmt.Sscan(m[1], &x)
			fmt.Sscan(m[2], &y)
			return x, y, nil
		}
	}
	return 0, 0, fmt.Errorf("monitor %q not found; connected: %s", name, strings.Join(seen, ", "))
}

func findBrowser() string {
	for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "windows":
		return `C:\Program Files\Google\Chrome\Application\chrome.exe`
	}
	return ""
}

func openDisplay(url string, x, y int) error {
	b := findBrowser()
	if b == "" {
		return fmt.Errorf("no Chrome/Chromium found; open %s manually", url)
	}
	dir, err := os.MkdirTemp("", "ai-speech-scroller-display-")
	if err != nil {
		return err
	}
	cmd := exec.Command(b,
		"--user-data-dir="+dir, "--no-first-run", "--no-default-browser-check",
		fmt.Sprintf("--window-position=%d,%d", x, y), "--kiosk", "--app="+url)
	return cmd.Start()
}

func openControl(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "listen address (keep it on loopback: the mic page needs a secure context)")
	monitor := flag.String("monitor", "", "open the teleprompter fullscreen on this monitor name (Linux, e.g. HDMI-1)")
	at := flag.String("display-at", "", "open the teleprompter fullscreen at screen position X,Y")
	noOpen := flag.Bool("no-open", false, "do not open any windows")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	h := newHub()
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/events", h.handleEvents)
	mux.HandleFunc("/msg", h.handleMsg)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	fmt.Printf("ai-speech-scroller %s\n  control : %s/\n  display : %s/display.html\n", version, base, base)

	if !*noOpen {
		x, y, haveSpot := 0, 0, false
		if *monitor != "" {
			if x, y, err = monitorOrigin(*monitor); err != nil {
				log.Printf("display not opened: %v", err)
			} else {
				haveSpot = true
			}
		} else if *at != "" {
			if _, err = fmt.Sscanf(*at, "%d,%d", &x, &y); err != nil {
				log.Printf("bad -display-at %q: want X,Y", *at)
			} else {
				haveSpot = true
			}
		}
		if haveSpot {
			if err := openDisplay(base+"/display.html", x, y); err != nil {
				log.Printf("display not opened: %v", err)
			}
		}
		openControl(base + "/")
	}
	log.Fatal(http.Serve(ln, mux))
}
