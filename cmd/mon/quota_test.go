package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestQuotaParse(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("MON_DIR", dir)
	defer os.Unsetenv("MON_DIR")

	// grava um quota.json no formato que o `mon quota` produz
	raw := `{"time":"2026-07-06T09:00:00-03:00","raw":{
		"five_hour":{"utilization":0.62,"resets_at":"2026-07-06T15:00:00Z"},
		"seven_day":{"utilization":88}}}`
	if err := os.WriteFile(filepath.Join(dir, "quota.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	q := readQuota()
	if !q.Present {
		t.Fatal("esperava Present=true")
	}
	if q.FiveHour < 61 || q.FiveHour > 63 {
		t.Errorf("five_hour: esperava ~62, veio %v", q.FiveHour)
	}
	if q.SevenDay < 87 || q.SevenDay > 89 {
		t.Errorf("seven_day: esperava ~88, veio %v", q.SevenDay)
	}
	if q.Reset.IsZero() {
		t.Error("esperava reset time parseado")
	}
	if q.Blocked {
		t.Error("não deveria estar bloqueado")
	}

	// agora um estado bloqueado
	raw2 := `{"time":"2026-07-06T09:00:00-03:00","raw":{"five_hour":{"utilization":100},"status":"rejected"}}`
	os.WriteFile(filepath.Join(dir, "quota.json"), []byte(raw2), 0o644)
	if q2 := readQuota(); !q2.Blocked {
		t.Error("esperava Blocked=true com status rejected")
	}
}

func TestViewHeight(t *testing.T) {
	m := newModel()
	m.width, m.height = 48, 18
	m.now = time.Now()
	m.sessions = []*Session{
		{ID: "1", Project: "a", Kind: KindWorking, LastSeen: m.now},
		{ID: "2", Project: "b", Kind: KindDone, LastSeen: m.now},
	}
	m.quota = Quota{Present: true, FiveHour: 30, SevenDay: 40}
	got := len(strings.Split(m.View(), "\n"))
	if got != 18 {
		t.Errorf("View deve ter 18 linhas p/ height=18, tem %d", got)
	}
}

func TestQuotaRealRender(t *testing.T) {
	m := newModel()
	m.width, m.height = 48, 18
	// "now" = horário do quota.json real
	m.now = time.Unix(1783343091, 0) // 2026-07-06 10:24:51 -03
	m.quota = Quota{
		Present:    true,
		FiveHour:   25,
		SevenDay:   4,
		Reset:      time.Unix(1783348200, 0),
		SevenReset: time.Unix(1783882800, 0),
	}
	m.animateQuota() // constrói as barras (gradiente da faixa)
	lines := m.quotaLines(48)
	if len(lines) != 2 {
		t.Fatalf("esperava 2 linhas de quota, veio %d", len(lines))
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w != 48 {
			t.Errorf("linha de quota deve ter 48 cols, tem %d: %q", w, l)
		}
		t.Logf("|%s|", l)
	}
}

func TestBandOf(t *testing.T) {
	cases := map[float64]int{-1: 0, 0: 0, 25: 0, 69.9: 0, 70: 1, 80: 1, 89.9: 1, 90: 2, 95: 2, 100: 2}
	for p, want := range cases {
		if got := bandOf(p); got != want {
			t.Errorf("bandOf(%.1f) = %d, want %d", p, got, want)
		}
	}
}

func TestEaseConverges(t *testing.T) {
	v := 0.0
	for i := 0; i < 100 && v != 1.0; i++ {
		v = ease(v, 1.0)
	}
	if v != 1.0 {
		t.Errorf("ease não convergiu p/ 1.0: %v", v)
	}
	// deve andar na direção do alvo e ficar no intervalo
	if got := ease(0.2, 0.8); got <= 0.2 || got >= 0.8 {
		t.Errorf("ease(0.2,0.8) fora do esperado: %v", got)
	}
}

// TestQuotaAnimates: a fração exibida se aproxima do alvo a cada tick, e a
// faixa (gradiente) acompanha o valor.
func TestQuotaAnimates(t *testing.T) {
	m := newModel()
	m.quota = Quota{Present: true, FiveHour: 95, SevenDay: 30}
	first := m.bar5
	m.animateQuota()
	if m.bar5 <= first {
		t.Errorf("bar5 deveria subir rumo a 0.95, ficou em %v", m.bar5)
	}
	if m.band5 != 2 {
		t.Errorf("faixa do 5h deveria ser 2 (>=90%%), veio %d", m.band5)
	}
	if m.band7 != 0 {
		t.Errorf("faixa do 7d deveria ser 0 (<70%%), veio %d", m.band7)
	}
}
