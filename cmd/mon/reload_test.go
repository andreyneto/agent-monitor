package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeEvents(t *testing.T, dir string, evs []Event) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, e := range evs {
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
	}
}

func hasSession(m model, id string) bool {
	for _, s := range m.sessions {
		if s.ID == id {
			return true
		}
	}
	return false
}

// TestStuckAttentionAges cobre o bug do card preso em "precisa de você": quando
// a permissão é resolvida fora da nossa visão (id da sessão muda, Claude fecha
// sem Stop, ferramenta longa), nenhum hook limpa o estado. A attention antiga
// deve envelhecer com o StaleMinutes; só "working" fica imune ao tempo.
func TestStuckAttentionAges(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("MON_DIR", dir)
	defer os.Unsetenv("MON_DIR")
	now := time.Now()

	writeEvents(t, dir, []Event{
		{Session: "old", Project: "p1", Kind: KindAttention, Message: "Claude needs your permission", Time: now.Add(-2 * time.Hour)},
		{Session: "work", Project: "p2", Kind: KindWorking, Time: now.Add(-2 * time.Hour)},
	})
	m := newModel()
	m.now = now
	m.cfg.StaleMinutes = 45
	m.reload()
	if hasSession(m, "old") {
		t.Error("attention de 2h deveria ter sumido (stale)")
	}
	if !hasSession(m, "work") {
		t.Error("working de 2h deveria permanecer (imune ao tempo)")
	}

	// attention recente aparece; 'c' consegue limpar uma attention presa
	writeEvents(t, dir, []Event{
		{Session: "new", Project: "p3", Kind: KindAttention, Message: "Claude needs your permission", Time: now.Add(-time.Minute)},
	})
	m.reload()
	if !hasSession(m, "new") {
		t.Fatal("attention recente deveria aparecer")
	}
	m.clearedAt = now
	m.reload()
	if hasSession(m, "new") {
		t.Error("'c' deveria limpar a attention presa")
	}
}
