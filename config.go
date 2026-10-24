package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is remembered between runs in the user config directory.
type Config struct {
	Mic      string `json:"mic"`     // microphone name ("" = system default)
	Display  string `json:"display"` // display name
	Size     int    `json:"size"`    // text size in px
	Column   int    `json:"column"`  // text column width, % of screen
	Guide    int    `json:"guide"`   // reading line, % from the top
	Mirror   bool   `json:"mirror"`  // flip horizontally for beam-splitter glass
	Script   string `json:"script"`  // last script path
	NoListen bool   `json:"no_listen"`
	EditSize int    `json:"edit_size"` // script editor text size in px
}

func defaultConfig() Config {
	return Config{Size: 72, Column: 80, Guide: 38, Mirror: false, EditSize: 32}
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ai-speech-scroller", "config.json")
}

func loadConfig() Config {
	c := defaultConfig()
	if p := configPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &c)
		}
	}
	return c
}

func (c Config) save() {
	p := configPath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}
