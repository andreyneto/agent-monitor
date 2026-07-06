package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TestSnapshot renderiza o painel com dados fixos p/ inspeção visual.
// Rode com:  go test -run Snapshot -v
func TestSnapshot(t *testing.T) {
	lipgloss.SetColorProfile(1) // força ANSI mesmo sem TTY (p/ inspecionar cores)
	base := time.Date(2026, 7, 6, 14, 32, 5, 0, time.Local)
	m := newModel()
	m.cfg.Sound = false
	m.width, m.height = 48, 18
	m.now = base
	m.sessions = []*Session{
		{ID: "1", Project: "bp-athena", Title: "corrigindo o fluxo de login com OAuth", Kind: KindAttention, LastSeen: base.Add(-12 * time.Second)},
		{ID: "2", Project: "fmoney", Title: "refatorando o cálculo de juros", Kind: KindWorking, LastSeen: base.Add(-64 * time.Second)},
		{ID: "3", Project: "copa-do-mundo-app", Title: "tela de tabela de jogos", Kind: KindWorking, LastSeen: base.Add(-9 * time.Second)},
		{ID: "4", Project: "lumina", Title: "", Kind: KindDone, LastSeen: base.Add(-3 * time.Minute)},
		{ID: "b5272305-abc", Project: "monitor", Title: "", Kind: KindStart, LastSeen: base.Add(-30 * time.Second)},
	}
	m.quota = Quota{Present: true, FiveHour: 62, SevenDay: 88}
	for _, fr := range []int{0, 1} {
		m.frame = fr
		fmt.Printf("\n===== FRAME %d (quota normal) =====\n", fr)
		fmt.Println("|--- 48 cols ---------------------------------|")
		fmt.Print(m.View())
		fmt.Println("\n|---------------------------------------------|")
	}

	m.quota = Quota{Present: true, Blocked: true, Reset: base.Add(37 * time.Minute)}
	for _, fr := range []int{0, 1} {
		m.frame = fr
		fmt.Printf("\n===== FRAME %d (quota BLOQUEADA) =====\n", fr)
		fmt.Print(m.View())
	}
}

func TestOverlays(t *testing.T) {
	m := newModel()
	m.width, m.height = 48, 18
	m.now = time.Now()
	fmt.Println("\n===== AJUDA (?) =====")
	m.mode = viewHelp
	fmt.Print(m.View())
	fmt.Println("\n===== SETTINGS (s), cursor no item 2 =====")
	m.mode = viewSettings
	m.setCursor = 2
	fmt.Print(m.View())
	fmt.Println()
}
