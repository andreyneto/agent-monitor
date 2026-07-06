package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func configPath() string { return filepath.Join(monDir(), "config.json") }

// Config são as preferências persistidas em ~/.claude/monitor/config.json.
type Config struct {
	Sound         bool   `json:"sound"`          // tocar som no alarme
	SoundPath     string `json:"sound_path"`     // qual som (afplay)
	StaleMinutes  int    `json:"stale_minutes"`  // sessão ociosa some depois de N min
	ShowQuota     bool   `json:"show_quota"`     // mostrar barras de quota
	Blink         bool   `json:"blink"`          // piscar linhas em alarme
	Clock24h      bool   `json:"clock_24h"`      // relógio 24h (senão 12h)
	Layout        string `json:"layout"`         // "list" | "grid" | "auto"
	GridCols      int    `json:"grid_cols"`      // colunas no grid manual (1..6)
	SpinnerStyle  string `json:"spinner_style"`  // estilo do indicador "trabalhando"
	OnlyAttention bool   `json:"only_attention"` // filtro: só "precisa de você"
}

func defaultConfig() Config {
	return Config{
		Sound:        true,
		SoundPath:    "/System/Library/Sounds/Glass.aiff",
		StaleMinutes: 45,
		ShowQuota:    true,
		Blink:        true,
		Clock24h:     true,
		Layout:       layoutAuto,
		GridCols:     2,
		SpinnerStyle: spinnerDefault,
	}
}

// loadConfig lê o config.json; campos ausentes ficam no default. Aplica também
// os overrides de env (MON_SOUND / MON_SILENT) por compatibilidade.
func loadConfig() Config {
	c := defaultConfig()
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c) // merge sobre os defaults
	}
	if v := os.Getenv("MON_SOUND"); v != "" {
		c.SoundPath = v
	}
	if os.Getenv("MON_SILENT") != "" {
		c.Sound = false
	}
	// normaliza campos novos (config.json antigo pode não tê-los)
	if c.Layout != layoutList && c.Layout != layoutGrid && c.Layout != layoutAuto {
		c.Layout = layoutAuto
	}
	if c.GridCols < 1 {
		c.GridCols = 2
	}
	if c.GridCols > maxGridCols {
		c.GridCols = maxGridCols
	}
	if !validSpinner(c.SpinnerStyle) {
		c.SpinnerStyle = spinnerDefault
	}
	return c
}

// saveConfig grava o config.json (troca atômica).
func saveConfig(c Config) error {
	if err := os.MkdirAll(monDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}
