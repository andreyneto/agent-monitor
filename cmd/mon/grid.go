package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---- modos de visualização -------------------------------------------------

const (
	layoutList = "list" // lista (2 linhas por sessão) — o modo original
	layoutGrid = "grid" // cards em N colunas fixas (GridCols)
	layoutAuto = "auto" // decide grid×lista e nº de colunas pela quantidade
)

const (
	maxGridCols  = 6  // teto do grid manual
	minCardOuter = 22 // largura mínima de um card (borda incluída) p/ caber
	cardH        = 3  // linhas de conteúdo por card (borda soma +2)
	rowsPerCol   = 3  // "sempre até 3 por coluna" no modo auto
)

// cardBlock = cardH + borda (topo/base).
func cardBlockH() int { return cardH + 2 }

// nextLayout cicla lista → grid → auto → lista.
func nextLayout(cur string) string {
	switch cur {
	case layoutList:
		return layoutGrid
	case layoutGrid:
		return layoutAuto
	default:
		return layoutList
	}
}

func layoutLabel(l string) string {
	switch l {
	case layoutList:
		return "lista"
	case layoutGrid:
		return "grid"
	default:
		return "auto"
	}
}

// maxColsFor devolve quantas colunas de card cabem na largura w.
func maxColsFor(w int) int {
	c := w / minCardOuter
	if c < 1 {
		c = 1
	}
	if c > maxGridCols {
		c = maxGridCols
	}
	return c
}

// resolveLayout decide, a partir da config e do tamanho, se renderiza como
// lista ou grid — e, no caso de grid, com quantas colunas.
//
// No modo "auto": 1 sessão vira um card cheio; a partir daí monta colunas com
// até 3 cards cada (cols = ceil(n/3)), crescendo até o limite que cabe na
// largura. Passou do limite, cai de volta pra lista (que rola com as setas).
func (m model) resolveLayout(w, bodyH int) (kind string, cols int) {
	n := len(m.sessions)
	switch m.cfg.Layout {
	case layoutList:
		return layoutList, 0
	case layoutGrid:
		cols = m.cfg.GridCols
		if cols < 1 {
			cols = 1
		}
		if mx := maxColsFor(w); cols > mx {
			cols = mx // não deixa o card ficar menor que o mínimo legível
		}
		if bodyH < cardBlockH() {
			return layoutList, 0 // nem um card cabe na altura → lista
		}
		return layoutGrid, cols
	default: // auto
		if n == 0 {
			return layoutList, 0
		}
		if bodyH < cardBlockH() {
			return layoutList, 0
		}
		cols = (n + rowsPerCol - 1) / rowsPerCol // ceil(n/3)
		if mx := maxColsFor(w); cols > mx {
			return layoutList, 0 // passou do limite de colunas → volta pra lista
		}
		return layoutGrid, cols
	}
}

// maxScroll é o maior offset de rolagem válido pro corpo atual.
func (m model) maxScroll(w int) int {
	_, bodyH := m.layoutDims(w)
	n := len(m.sessions)
	if n == 0 {
		return 0
	}
	kind, cols := m.resolveLayout(w, bodyH)
	if kind == layoutList {
		vis := bodyH / 2
		if vis < 1 {
			vis = 1
		}
		return max0(n - vis)
	}
	if cols == 1 && n == 1 {
		return 0 // card cheio, nada pra rolar
	}
	per := cardBlockH()
	visRows := bodyH / per
	if visRows < 1 {
		visRows = 1
	}
	totalRows := (n + cols - 1) / cols
	return max0(totalRows - visRows)
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// ---- render do corpo -------------------------------------------------------

// renderBody devolve exatamente bodyH linhas com o conteúdo central do painel:
// stats (se vazio), lista ou grid.
func (m model) renderBody(w, bodyH int) []string {
	if len(m.sessions) == 0 {
		return m.renderStats(w, bodyH)
	}
	kind, cols := m.resolveLayout(w, bodyH)
	if kind == layoutList {
		return m.renderList(w, bodyH)
	}
	return m.renderGrid(w, bodyH, cols)
}

// renderList é o layout original (2 linhas por sessão), agora com rolagem.
func (m model) renderList(w, bodyH int) []string {
	vis := bodyH / 2
	if vis < 1 {
		vis = 1
	}
	start := clampScroll(m.scroll, m.sessions, vis, false, 0)
	var out []string
	for i := start; i < len(m.sessions) && len(out) < vis*2; i++ {
		row := m.renderRow(m.sessions[i], w)
		out = append(out, strings.Split(row, "\n")...)
	}
	return padLines(out, bodyH)
}

// renderGrid desenha os cards em `cols` colunas, com rolagem por linha de cards.
func (m model) renderGrid(w, bodyH, cols int) []string {
	n := len(m.sessions)
	if cols == 1 && n == 1 {
		return m.renderFullCard(m.sessions[0], w, bodyH)
	}

	gap := 1
	// reserva ~2 colunas de margem (a borda dos cards não encosta na beira
	// curva do CRT). Centraliza o bloco no que sobra.
	avail := w - 2
	if avail < minCardOuter {
		avail = w
	}
	colOuter := (avail - (cols-1)*gap) / cols
	if colOuter < minCardOuter {
		colOuter = minCardOuter
	}
	used := cols*colOuter + (cols-1)*gap
	lead := strings.Repeat(" ", max0((w-used)/2))

	per := cardBlockH()
	visRows := bodyH / per
	if visRows < 1 {
		visRows = 1
	}
	totalRows := (n + cols - 1) / cols
	start := clampScroll(m.scroll, m.sessions, visRows, true, cols)

	var out []string
	for r := start; r < totalRows && len(out) < visRows*per; r++ {
		// monta os cards desta linha e junta lado a lado
		var cards [][]string
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			if idx >= n {
				cards = append(cards, blankCard(colOuter, per))
				continue
			}
			cards = append(cards, m.cardLines(m.sessions[idx], colOuter, per))
		}
		for _, ln := range joinCardsRow(cards, gap) {
			out = append(out, lead+ln)
		}
	}
	return padLines(out, bodyH)
}

// clampScroll limita o offset ao intervalo válido. Se grid, offset é em linhas
// de cards (cols por linha); se lista, em sessões.
func clampScroll(scroll int, sessions []*Session, vis int, grid bool, cols int) int {
	n := len(sessions)
	var maxS int
	if grid {
		totalRows := (n + cols - 1) / cols
		maxS = max0(totalRows - vis)
	} else {
		maxS = max0(n - vis)
	}
	if scroll < 0 {
		return 0
	}
	if scroll > maxS {
		return maxS
	}
	return scroll
}

// cardLines renderiza um card de sessão como bloco de `outerH` linhas, cada uma
// com `outer` colunas de largura (borda incluída).
func (m model) cardLines(s *Session, outer, outerH int) []string {
	subtitle := s.Title
	if subtitle == "" {
		subtitle = "#" + shortID(s.ID)
	}
	if s.Kind == KindBackground {
		if bs := bgSummary(s); bs != "" {
			subtitle = bs
		}
	}
	ago := agoLabel(s.LastSeen, m.now)

	// "precisa de você": o card INTEIRO pisca (fundo vermelho ↔ contorno), como
	// a linha no modo lista — a borda sozinha não chamava atenção suficiente.
	if s.Kind == KindAttention {
		return m.alarmCardLines(s, subtitle, ago, outer, outerH)
	}
	// aviso de "pronto": card inteiro em ciano na fase acesa (pisca por frames)
	if n := m.doneFlash[s.ID]; n > 0 && m.doneFlashOn(n) {
		return m.doneFlashCardLines(s, subtitle, ago, outer, outerH)
	}

	var icon, label string
	var style lipgloss.Style
	switch s.Kind {
	case KindAttention:
		icon, label, style = "▲", "precisa de você", stAtten
	case KindWorking:
		icon = m.spin.View()
		label, style = "trabalhando", stWorking
	case KindBackground:
		icon = m.spin.View()
		label, style = "em background", stBack
	case KindDone:
		icon, label, style = "✓", "pronto", stDone
	case KindStart:
		icon, label, style = "○", "iniciando", stIdle
	default:
		icon, label, style = "·", "ocioso", stIdle
	}

	// innerW = largura do conteúdo dentro das bordas; tw = texto com 1 col de
	// respiro de cada lado. Montamos cada linha JÁ com tw colunas pra o lipgloss
	// nunca precisar quebrar (o que cortaria texto por causa do Height).
	innerW := outer - 2
	tw := innerW - 2
	if tw < 4 {
		tw = 4
	}

	projStyle := stProject.Foreground(sessionColor(s.ID))
	projMax := tw - 2 - lipgloss.Width(ago) - 1
	if projMax < 3 {
		projMax = 3
	}
	head := style.Render(icon) + " " + projStyle.Render(trunc(s.Project, projMax))
	line1 := " " + spread(head, stDim.Render(ago), tw) + " "
	line2 := " " + padRight(style.Render(trunc(label, tw)), tw) + " "
	line3 := " " + padRight(stSubtitle.Render(trunc(subtitle, tw)), tw) + " "
	content := line1 + "\n" + line2 + "\n" + line3

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(sessionColor(s.ID)).
		Width(innerW).
		Height(cardH).
		Render(content)

	return padCardWidth(strings.Split(box, "\n"), outer, outerH)
}

// alarmCardLines desenha o card de "precisa de você" piscando o card INTEIRO:
// no frame "aceso", fundo vermelho sólido com texto claro (igual à barra da
// lista); no frame "apagado", contorno + texto vermelhos sobre fundo padrão.
// Se o piscar está desligado, fica sempre aceso (vermelho sólido).
func (m model) alarmCardLines(s *Session, subtitle, ago string, outer, outerH int) []string {
	innerW := outer - 2
	tw := innerW - 2
	if tw < 4 {
		tw = 4
	}

	projMax := tw - 2 - lipgloss.Width(ago) - 1
	if projMax < 3 {
		projMax = 3
	}
	// linhas em texto puro (a cor é aplicada na linha toda), largura innerW
	head := "▲ " + trunc(s.Project, projMax)
	l1 := " " + spread(head, ago, tw) + " "
	l2 := " " + padRight("precisa de você", tw) + " "
	l3 := " " + padRight(trunc(subtitle, tw), tw) + " "

	on := m.blink()
	textSt := stAlarmOff // vermelho sobre fundo padrão
	if on {
		textSt = stAlarmOn // texto claro sobre fundo vermelho
	}
	content := textSt.Render(l1) + "\n" + textSt.Render(l2) + "\n" + textSt.Render(l3)

	bs := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Width(innerW).
		Height(cardH)
	if on {
		// contorno claro sobre fundo vermelho → card vira um bloco vermelho cheio
		bs = bs.BorderForeground(lipgloss.Color("231")).BorderBackground(lipgloss.Color("196"))
	}
	box := bs.Render(content)
	return padCardWidth(strings.Split(box, "\n"), outer, outerH)
}

// doneFlashCardLines desenha o card de "pronto" aceso (ciano cheio, texto
// escuro) — a fase "acesa" do aviso que pisca por doneFlashFrames.
func (m model) doneFlashCardLines(s *Session, subtitle, ago string, outer, outerH int) []string {
	innerW := outer - 2
	tw := innerW - 2
	if tw < 4 {
		tw = 4
	}
	projMax := tw - 2 - lipgloss.Width(ago) - 1
	if projMax < 3 {
		projMax = 3
	}
	head := "✓ " + trunc(s.Project, projMax)
	l1 := " " + spread(head, ago, tw) + " "
	l2 := " " + padRight("pronto", tw) + " "
	l3 := " " + padRight(trunc(subtitle, tw), tw) + " "
	content := stDoneFlash.Render(l1) + "\n" + stDoneFlash.Render(l2) + "\n" + stDoneFlash.Render(l3)

	bs := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(innerW).
		Height(cardH).
		BorderForeground(lipgloss.Color("51")).BorderBackground(lipgloss.Color("51"))
	box := bs.Render(content)
	return padCardWidth(strings.Split(box, "\n"), outer, outerH)
}

// renderFullCard usa a tela toda pra uma única sessão — versão "detalhada".
func (m model) renderFullCard(s *Session, w, bodyH int) []string {
	subtitle := s.Title
	if subtitle == "" {
		subtitle = "#" + shortID(s.ID)
	}
	if s.Kind == KindBackground {
		if bs := bgSummary(s); bs != "" {
			subtitle = bs
		}
	}
	var icon, label string
	var style lipgloss.Style
	attention := s.Kind == KindAttention
	flash := !attention && m.doneFlash[s.ID] > 0 && m.doneFlashOn(m.doneFlash[s.ID])
	switch s.Kind {
	case KindAttention:
		icon, label, style = "▲", "PRECISA DE VOCÊ", stAtten
	case KindWorking:
		icon = m.spin.View()
		label, style = "trabalhando", stWorking
	case KindBackground:
		icon = m.spin.View()
		label, style = "em background", stBack
	case KindDone:
		icon, label, style = "✓", "pronto", stDone
	case KindStart:
		icon, label, style = "○", "iniciando", stIdle
	default:
		icon, label, style = "·", "ocioso", stIdle
	}

	// boxW = Width do lipgloss. Deixamos ~3 colunas de margem de cada lado
	// (borda soma +2) pra não desenhar a borda na beira curva do CRT, onde
	// ela some/serrilha ("travado meio fora"). tw = texto útil (menos padding).
	boxW := w - 8
	if boxW > 80 {
		boxW = 80
	}
	if boxW < 12 {
		boxW = 12
	}
	tw := boxW - 4
	if tw < 8 {
		tw = 8
	}
	footer := trunc(fmt.Sprintf("%s · visto há %s", s.Source, agoLabel(s.LastSeen, m.now)), tw)
	var inner []string
	if attention {
		// texto puro: a cor vem do estilo do card inteiro (que pisca)
		inner = []string{"▲  " + trunc(s.Project, tw-4), "", "PRECISA DE VOCÊ", "", trunc(subtitle, tw)}
		if s.Message != "" && s.Message != subtitle {
			inner = append(inner, "", trunc(s.Message, tw))
		}
		inner = append(inner, "", footer)
	} else if flash {
		// texto puro: a cor vem do estilo do card inteiro (pulso ciano)
		inner = []string{"✓  " + trunc(s.Project, tw-4), "", "PRONTO", "", trunc(subtitle, tw)}
		if s.Message != "" && s.Message != subtitle {
			inner = append(inner, "", trunc(s.Message, tw))
		}
		inner = append(inner, "", footer)
	} else {
		projStyle := stProject.Foreground(sessionColor(s.ID))
		inner = []string{
			style.Render(icon) + "  " + projStyle.Render(trunc(s.Project, tw-4)),
			"",
			style.Render(trunc(label, tw)),
			"",
			stSubtitle.Render(trunc(subtitle, tw)),
		}
		if s.Message != "" && s.Message != subtitle {
			inner = append(inner, "", stDim.Render(trunc(s.Message, tw)))
		}
		inner = append(inner, "", stDim.Render(footer))
	}

	ch := bodyH // ocupa toda a altura do corpo
	if ch < len(inner)+2 {
		ch = len(inner) + 2
	}
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(boxW).
		Height(ch - 2)
	switch {
	case attention && m.blink():
		// card INTEIRO vermelho sólido, texto claro (como a barra da lista)
		boxStyle = boxStyle.Bold(true).
			Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196")).
			BorderForeground(lipgloss.Color("231")).BorderBackground(lipgloss.Color("196"))
	case attention:
		// frame "apagado": contorno + texto vermelhos sobre fundo padrão
		boxStyle = boxStyle.Bold(true).
			Foreground(lipgloss.Color("196")).BorderForeground(lipgloss.Color("196"))
	case flash:
		// pulso de "pronto": card inteiro ciano, texto escuro
		boxStyle = boxStyle.Bold(true).
			Foreground(lipgloss.Color("16")).Background(lipgloss.Color("51")).
			BorderForeground(lipgloss.Color("51")).BorderBackground(lipgloss.Color("51"))
	default:
		boxStyle = boxStyle.BorderForeground(sessionColor(s.ID))
	}
	box := boxStyle.Render(strings.Join(inner, "\n"))
	// centraliza o card na largura (a borda já soma +2 ao boxW)
	lead := strings.Repeat(" ", max0((w-(boxW+2))/2))
	lines := strings.Split(box, "\n")
	for i, ln := range lines {
		lines[i] = lead + ln
	}
	return padLines(lines, bodyH)
}

// blankCard devolve um card "buraco" (só espaços) pra completar a última linha.
func blankCard(outer, outerH int) []string {
	blank := strings.Repeat(" ", outer)
	out := make([]string, outerH)
	for i := range out {
		out[i] = blank
	}
	return out
}

// joinCardsRow junta N blocos de card lado a lado, com `gap` espaços entre eles.
func joinCardsRow(cards [][]string, gap int) []string {
	if len(cards) == 0 {
		return nil
	}
	h := 0
	for _, c := range cards {
		if len(c) > h {
			h = len(c)
		}
	}
	sep := strings.Repeat(" ", gap)
	out := make([]string, h)
	for i := 0; i < h; i++ {
		var parts []string
		for _, c := range cards {
			if i < len(c) {
				parts = append(parts, c[i])
			}
		}
		out[i] = strings.Join(parts, sep)
	}
	return out
}

// padCardWidth garante que o bloco tenha `outerH` linhas de `outer` colunas.
func padCardWidth(lines []string, outer, outerH int) []string {
	out := make([]string, 0, outerH)
	for i := 0; i < outerH; i++ {
		if i < len(lines) {
			out = append(out, padRight(lines[i], outer))
		} else {
			out = append(out, strings.Repeat(" ", outer))
		}
	}
	return out
}

// padLines completa (ou trunca) a lista pra exatamente h linhas.
func padLines(lines []string, h int) []string {
	if len(lines) > h {
		return lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

// ---- tela vazia: uso geral do Claude Code ----------------------------------

// heatPalette: 0 = dia sem atividade (cinza bem escuro, pra o laranja saltar),
// 1..4 = intensidade crescente em LARANJA (base do painel), separados por brilho
// pra ler bem no fósforo do cool-retro-term. Glifo sempre "█" (largura 1 estável).
var heatPalette = []lipgloss.Color{"236", "130", "166", "208", "214"}

// renderStats desenha o Overview de uso do Claude Code (heatmap estilo GitHub +
// bloco de estatísticas), ocupando bodyH linhas. Aparece quando não há sessão.
func (m model) renderStats(w, bodyH int) []string {
	u := m.usage
	if !u.Present {
		var rows []string
		rows = append(rows, center(stProject.Render("sem dados de uso ainda"), w), "")
		rows = append(rows, center(stDim.Render("o Claude Code grava o uso em"), w))
		rows = append(rows, center(stDim.Render("~/.claude/stats-cache.json"), w))
		return vcenter(rows, bodyH)
	}

	d := u.derive(m.now)
	heat, blockW := m.renderHeatmap(u, w)
	// ancora o grid à direita (semana atual encosta na margem direita; meses
	// vazios ficam à esquerda), com ~2 cols de margem do CRT.
	heatLead := strings.Repeat(" ", max0(w-2-blockW))

	var rows []string
	// nota de defasagem no topo (sempre visível): o cache só recomputa no /usage
	if note := u.staleNote(m.now); note != "" {
		rows = append(rows, center(stDim.Render(note), w), "")
	}
	for _, ln := range heat {
		rows = append(rows, heatLead+ln)
	}

	// legenda menos → mais (explica as cores) — incluída só se ainda sobrar
	// espaço pra pelo menos 3 linhas de estatística embaixo.
	if len(rows)+2+4 <= bodyH {
		var lg strings.Builder
		lg.WriteString(stDim.Render("menos "))
		for _, c := range heatPalette {
			lg.WriteString(lipgloss.NewStyle().Foreground(c).Render("█"))
		}
		lg.WriteString(stDim.Render(" mais"))
		rows = append(rows, "", heatLead+lg.String())
	}

	// bloco de estatísticas: inclui quantas linhas couberem (prioriza os números)
	statLines := m.statBlock(u, d, w)
	if maxStat := bodyH - len(rows) - 1; maxStat > 0 && len(statLines) > 0 {
		if len(statLines) > maxStat {
			statLines = statLines[:maxStat]
		}
		rows = append(rows, "")
		rows = append(rows, statLines...)
	}

	return vcenter(rows, bodyH)
}

// statBlock monta as linhas de estatística (modelo favorito, tokens, sessões,
// sequências, etc.), centralizadas na largura w — em 2 colunas quando cabe.
func (m model) statBlock(u Usage, d derived, w int) []string {
	type kv struct{ k, v string }
	items := []kv{
		{"Modelo favorito", capWord(u.Favorite)},
		{"Total de tokens", abbrevTokens(u.TotalTokens)},
		{"Sessões", groupInt(u.TotalSessions)},
		{"Sessão + longa", durLabel(u.LongestMs)},
		{"Dias ativos", fmt.Sprintf("%d/%d", d.activeDays, d.spanDays)},
		{"Maior sequência", fmt.Sprintf("%d dias", d.longest)},
		{"Dia + ativo", dateShort(d.mostDate)},
		{"Sequência atual", fmt.Sprintf("%d dias", d.current)},
	}
	// "label: valor" alinhado à esquerda (estilo do /usage)
	cell := func(it kv) string { return stDim.Render(it.k+": ") + stProject.Render(it.v) }

	statW := w - 4
	if statW > 60 {
		statW = 60
	}
	if statW < 18 {
		statW = 18
	}
	lead := strings.Repeat(" ", max0((w-statW)/2))

	// 2 colunas em telas largas; senão 1 coluna (valor alinhado à direita)
	if w >= 64 {
		gutter := 3
		colW := (statW - gutter) / 2
		var out []string
		for i := 0; i+1 < len(items); i += 2 {
			out = append(out, lead+padRight(cell(items[i]), colW)+strings.Repeat(" ", gutter)+cell(items[i+1]))
		}
		return out
	}
	var out []string
	for _, it := range items {
		out = append(out, lead+spread(stDim.Render(it.k+": "), stProject.Render(it.v), statW))
	}
	return out
}

// renderHeatmap desenha o calendário de atividade estilo GitHub: colunas =
// semanas (domingo→sábado), linhas = dias da semana. Devolve as linhas (rótulo
// de meses + 7 linhas de dias) e a largura total do bloco.
func (m model) renderHeatmap(u Usage, w int) (lines []string, blockW int) {
	const leftW = 4    // "Seg " etc.
	const cellStep = 2 // célula + 1 espaço de respiro entre as semanas

	// preenche toda a largura disponível (com ~2 cols de margem de cada lado);
	// NÃO limita ao histórico — semanas sem dado ficam vazias à esquerda.
	avail := w - leftW - 4
	if avail < cellStep {
		avail = cellStep
	}
	weeks := avail / cellStep
	if weeks > 53 {
		weeks = 53
	}
	if weeks < 1 {
		weeks = 1
	}
	gridW := weeks*cellStep - 1 // sem espaço após a última coluna
	blockW = leftW + gridW

	today := dateOnly(m.now)
	curSun := today.AddDate(0, 0, -int(today.Weekday())) // domingo desta semana (coluna mais à direita)
	colSun := func(c int) time.Time { return curSun.AddDate(0, 0, -7*(weeks-1-c)) }

	// máximo pra escala (comprimido com raiz pra dias medianos não sumirem)
	maxN := 1
	for c := 0; c < weeks; c++ {
		for r := 0; r < 7; r++ {
			if n := u.Daily[colSun(c).AddDate(0, 0, r).Format("2006-01-02")]; n > maxN {
				maxN = n
			}
		}
	}

	// cabeçalho de meses: rótulo de 3 letras quando a semana muda de mês
	months := []string{"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}
	hdr := []rune(strings.Repeat(" ", gridW))
	last := -1
	lastEnd := -2 // última coluna ocupada por um rótulo (p/ não sobrepor)
	for c := 0; c < weeks; c++ {
		mth := int(colSun(c).Month())
		if mth == last {
			continue
		}
		last = mth
		// pula mês "sliver" na borda esquerda (só 1 coluna antes de virar)
		if c == 0 && weeks > 1 && int(colSun(1).Month()) != mth {
			continue
		}
		pos := c * cellStep
		lab := months[mth-1]
		if pos < lastEnd+2 || pos+len(lab) > gridW {
			continue // colidiria com o rótulo anterior ou não cabe
		}
		for i := 0; i < len(lab); i++ {
			hdr[pos+i] = rune(lab[i])
		}
		lastEnd = pos + len(lab) - 1
	}
	lines = append(lines, strings.Repeat(" ", leftW)+stDim.Render(string(hdr)))

	dayLabels := map[int]string{1: "Seg", 3: "Qua", 5: "Sex"}
	for r := 0; r < 7; r++ {
		var b strings.Builder
		b.WriteString(stDim.Render(padRight(dayLabels[r], leftW)))
		for c := 0; c < weeks; c++ {
			date := colSun(c).AddDate(0, 0, r)
			if date.After(today) {
				b.WriteByte(' ') // futuro (dias da semana atual ainda por vir)
			} else {
				lvl := heatLevel(u.Daily[date.Format("2006-01-02")], maxN)
				b.WriteString(lipgloss.NewStyle().Foreground(heatPalette[lvl]).Render("█"))
			}
			if c < weeks-1 {
				b.WriteByte(' ') // respiro entre as semanas
			}
		}
		lines = append(lines, b.String())
	}
	return lines, blockW
}

// heatLevel mapeia mensagens/dia → nível 0..4 (raiz comprime a escala).
func heatLevel(v, max int) int {
	if v <= 0 {
		return 0
	}
	f := math.Sqrt(float64(v) / float64(max))
	switch {
	case f >= 0.75:
		return 4
	case f >= 0.5:
		return 3
	case f >= 0.25:
		return 2
	default:
		return 1
	}
}

// vcenter centraliza verticalmente as linhas dentro de h (preenche com vazias).
func vcenter(lines []string, h int) []string {
	if len(lines) >= h {
		return lines[:h]
	}
	top := (h - len(lines)) / 2
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, lines...)
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// agoLabel formata um "há quanto tempo" curto: 5s, 12m, 3h, 2d.
func agoLabel(t, now time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
	switch {
	case d < time.Second:
		return "agora"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
