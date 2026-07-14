package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// eventos do Claude Code que queremos capturar.
var hookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"Notification",
	"PostToolUse",
	"SubagentStop",
	"Stop",
	"SessionEnd",
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// runInstall injeta os hooks no ~/.claude/settings.json (com backup).
func runInstall(args []string) int {
	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "não consegui achar o binário:", err)
		return 1
	}
	binPath, _ = filepath.Abs(binPath)
	cmd := binPath + " emit"

	sp := settingsPath()
	raw, err := os.ReadFile(sp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "não consegui ler", sp, ":", err)
		return 1
	}

	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		fmt.Fprintln(os.Stderr, "settings.json inválido:", err)
		return 1
	}

	// backup com timestamp
	bak := sp + ".bak." + time.Now().Format("20060102-150405")
	if err := os.WriteFile(bak, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "falha no backup:", err)
		return 1
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	added := 0
	for _, ev := range hookEvents {
		arr, _ := hooks[ev].([]any)
		if hookAlreadyPresent(arr, cmd) {
			continue
		}
		entry := map[string]any{
			"hooks": []any{
				map[string]any{"type": "command", "command": cmd},
			},
		}
		hooks[ev] = append(arr, entry)
		added++
	}
	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "falha ao serializar:", err)
		return 1
	}
	if err := os.WriteFile(sp, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "falha ao gravar:", err)
		return 1
	}

	fmt.Printf("✓ hooks instalados em %s (%d novos)\n", sp, added)
	fmt.Printf("  backup: %s\n", bak)
	fmt.Printf("  comando: %s\n", cmd)
	fmt.Println("\nReinicie as sessões do Claude Code para ativar.")
	return 0
}

// hookAlreadyPresent evita duplicar nosso comando num evento.
func hookAlreadyPresent(arr []any, cmd string) bool {
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := m["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if ok && hm["command"] == cmd {
				return true
			}
		}
	}
	return false
}
