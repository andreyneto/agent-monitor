package main

import "testing"

func sess(n int) []*Session {
	out := make([]*Session, n)
	for i := range out {
		out[i] = &Session{ID: string(rune('a' + i)), Kind: KindWorking}
	}
	return out
}

// TestAutoLayout cobre a regra do modo "auto": 1 card cheio, colunas com até 3
// cards cada crescendo até caber na largura, e queda pra lista ao estourar.
func TestAutoLayout(t *testing.T) {
	// largura 48 → cabem no máximo 2 colunas de card (minCardOuter=22)
	cases := []struct {
		n        int
		wantKind string
		wantCols int
	}{
		{1, layoutGrid, 1}, // 1 sessão: card cheio (1 coluna)
		{2, layoutGrid, 1}, // 2-3: uma coluna
		{3, layoutGrid, 1},
		{4, layoutGrid, 2}, // 4-6: duas colunas
		{6, layoutGrid, 2},
		{7, layoutList, 0}, // ceil(7/3)=3 > 2 colunas → volta pra lista
		{12, layoutList, 0},
	}
	for _, c := range cases {
		m := newModel()
		m.cfg.Layout = layoutAuto
		m.sessions = sess(c.n)
		kind, cols := m.resolveLayout(48, 16)
		if kind != c.wantKind || (kind == layoutGrid && cols != c.wantCols) {
			t.Errorf("n=%d: got (%s,%d), want (%s,%d)", c.n, kind, cols, c.wantKind, c.wantCols)
		}
	}
}

// TestAutoWiderScreen: numa tela larga cabem mais colunas antes de virar lista.
func TestAutoWiderScreen(t *testing.T) {
	m := newModel()
	m.cfg.Layout = layoutAuto
	m.sessions = sess(9) // ceil(9/3)=3 colunas
	kind, cols := m.resolveLayout(90, 16)
	if kind != layoutGrid || cols != 3 {
		t.Fatalf("9 sessões em 90 cols: got (%s,%d), want (grid,3)", kind, cols)
	}
}

// TestMaxScrollList garante que a rolagem para na última "tela".
func TestMaxScrollList(t *testing.T) {
	m := newModel()
	m.cfg.Layout = layoutList
	m.cfg.ShowQuota = false
	m.width, m.height = 48, 18
	m.sessions = sess(20)
	// bodyH = 18 - (header 1 + régua 1) - 2 = 14 → 7 sessões visíveis
	if got := m.maxScroll(48); got != 20-7 {
		t.Errorf("maxScroll lista: got %d, want %d", got, 20-7)
	}
}
