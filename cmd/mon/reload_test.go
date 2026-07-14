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

// TestRunningBgTasks: só conta tarefas com status "running"; descrição vazia
// vira placeholder.
func TestRunningBgTasks(t *testing.T) {
	got := runningBgTasks([]bgTask{
		{Status: "running", Description: "boot emulator"},
		{Status: "completed", Description: "build já terminou"},
		{Status: "running", Description: ""},
	})
	if len(got) != 2 {
		t.Fatalf("esperava 2 rodando, veio %d: %v", len(got), got)
	}
	if got[1] == "" {
		t.Error("descrição vazia deveria virar placeholder")
	}
}

// TestBackgroundState: sessão com tarefa em background fica imune ao tempo (como
// working, pois ainda há trabalho rodando), aparece acima de "done" na ordem, e
// carrega as descrições das tarefas pro Session.
func TestBackgroundState(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("MON_DIR", dir)
	defer os.Unsetenv("MON_DIR")
	now := time.Now()

	if pr(KindBackground) >= pr(KindDone) {
		t.Error("background deveria ter prioridade acima de done")
	}

	writeEvents(t, dir, []Event{
		{Session: "bg", Project: "p1", Kind: KindBackground, BgTasks: []string{"Boot Android emulator"}, Time: now.Add(-2 * time.Hour)},
		{Session: "done", Project: "p2", Kind: KindDone, Time: now.Add(-2 * time.Hour)},
	})
	m := newModel()
	m.now = now
	m.cfg.StaleMinutes = 45
	m.reload()

	if !hasSession(m, "bg") {
		t.Error("background de 2h deveria permanecer (tarefa ainda rodando)")
	}
	if hasSession(m, "done") {
		t.Error("done de 2h deveria ter sumido (stale)")
	}
	for _, s := range m.sessions {
		if s.ID == "bg" && len(s.BgTasks) != 1 {
			t.Errorf("BgTasks deveria ter 1 item, veio %v", s.BgTasks)
		}
	}
}

// TestDoneFlash cobre o pulso de "pronto": deve disparar quando a sessão ENTRA
// em done (transição), não a cada reload nem no estado pré-existente do boot,
// e respeitar o toggle AlertDone.
func TestDoneFlash(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("MON_DIR", dir)
	defer os.Unsetenv("MON_DIR")
	now := time.Now()

	// 1º reload com a sessão JÁ em done: só semeia o estado, não pisca.
	writeEvents(t, dir, []Event{
		{Session: "s1", Project: "p", Kind: KindDone, Time: now.Add(-time.Minute)},
	})
	m := newModel()
	m.now = now
	m.reload()
	if m.doneFlash["s1"] != 0 {
		t.Fatal("done pré-existente no boot não deveria piscar")
	}

	// transição working → done dispara o flash.
	writeEvents(t, dir, []Event{
		{Session: "s2", Project: "p", Kind: KindWorking, Time: now.Add(-time.Minute)},
	})
	m.reload() // vê s2 trabalhando (sem flash); s1 sumiu do log
	if m.doneFlash["s2"] != 0 {
		t.Fatal("working não deveria piscar")
	}
	writeEvents(t, dir, []Event{
		{Session: "s2", Project: "p", Kind: KindWorking, Time: now.Add(-time.Minute)},
		{Session: "s2", Project: "p", Kind: KindDone, Time: now.Add(-time.Second)},
	})
	m.reload()
	if m.doneFlash["s2"] != doneFlashFrames {
		t.Fatalf("transição p/ done deveria armar o flash (%d), veio %d", doneFlashFrames, m.doneFlash["s2"])
	}
	// reload de novo sem mudar o estado NÃO re-arma (senão ficaria piscando).
	m.doneFlash["s2"] = 2 // simula alguns frames já consumidos
	m.reload()
	if m.doneFlash["s2"] != 2 {
		t.Errorf("done→done não deveria re-armar o flash, veio %d", m.doneFlash["s2"])
	}

	// AlertDone desligado: nenhum flash.
	m2 := newModel()
	m2.now = now
	m2.cfg.AlertDone = false
	m2.reload() // boot semeia
	writeEvents(t, dir, []Event{
		{Session: "s2", Project: "p", Kind: KindWorking, Time: now.Add(-time.Minute)},
	})
	m2.reload()
	writeEvents(t, dir, []Event{
		{Session: "s2", Project: "p", Kind: KindWorking, Time: now.Add(-time.Minute)},
		{Session: "s2", Project: "p", Kind: KindDone, Time: now.Add(-time.Second)},
	})
	m2.reload()
	if m2.doneFlash["s2"] != 0 {
		t.Error("AlertDone desligado não deveria piscar")
	}
}
