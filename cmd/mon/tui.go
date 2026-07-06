package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- aparência ------------------------------------------------------------

var (
	stTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	stRule      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	stProject   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")) // negrito, claro
	stSubtitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))            // peso leve, apagado
	stAtten     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	stAlarmOn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196"))
	stAlarmOff  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	stQuotaOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // laranja vivo (< 70%)
	stQuotaWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // âmbar (70–90%)
	stWorking   = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	stDone      = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	stIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// ---- model ----------------------------------------------------------------

type viewMode int

const (
	viewNormal viewMode = iota
	viewHelp
	viewSettings
)

type filterMode int

const (
	filterAll filterMode = iota
	filterAttention
)

type model struct {
	sessions   []*Session
	quota      Quota
	now        time.Time
	width      int
	height     int
	frame      int             // contador de frames p/ o blink do alarme
	rung       map[string]bool // sessões que já tocaram o alarme
	quotaRung  bool            // já alarmou o bloqueio de quota?
	cfg        Config
	mode       viewMode
	filter     filterMode
	setCursor  int           // item selecionado na tela de settings
	clearedAt  time.Time     // sessões inativas antes disso ficam escondidas
	notice     string        // mensagem transitória no rodapé
	noticeTTL  int           // frames restantes da mensagem
	scroll     int           // offset de rolagem (sessões na lista, linhas no grid)
	usage      Usage         // uso geral do Claude Code (tela vazia)
	usageMtime time.Time     // mtime do stats-cache já parseado (cache)
	spin       spinner.Model // indicador de atividade (bubbles) do status "trabalhando"

	// barras de quota animadas (progress do bubbles). bar* é a fração exibida
	// (animada rumo ao valor real); band* é a faixa de cor atual do gradiente.
	prog5, prog7 progress.Model
	bar5, bar7   float64
	band5, band7 int
}

func (m *model) setNotice(s string) { m.notice = s; m.noticeTTL = 20 }

type tickMsg time.Time

func newModel() model {
	cfg := loadConfig()
	filter := filterAll
	if cfg.OnlyAttention {
		filter = filterAttention
	}
	return model{
		width:  48,
		height: 18,
		now:    time.Now(),
		rung:   map[string]bool{},
		cfg:    cfg,
		filter: filter,
		spin:   newSpinner(cfg.SpinnerStyle),
		band5:  -1, // força construir as barras na 1ª animação
		band7:  -1,
	}
}

func (m model) Init() tea.Cmd { return tea.Batch(tick(), m.spin.Tick) }

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.now = time.Time(msg)
		m.frame++
		if m.noticeTTL > 0 {
			m.noticeTTL--
			if m.noticeTTL == 0 {
				m.notice = ""
			}
		}
		m.reload()
		m.animateQuota()
		return m, tick()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case viewHelp:
		m.mode = viewNormal // qualquer tecla fecha a ajuda
		return m, nil

	case viewSettings:
		switch k {
		case "s", "esc", "q":
			_ = saveConfig(m.cfg)
			m.mode = viewNormal
			m.setNotice("config salva")
		case "up", "k":
			if m.setCursor > 0 {
				m.setCursor--
			}
		case "down", "j":
			if m.setCursor < len(settingItems)-1 {
				m.setCursor++
			}
		case "left", "right", " ", "enter", "h", "l":
			delta := 1
			if k == "left" || k == "h" {
				delta = -1
			}
			before := m.cfg.SpinnerStyle
			settingItems[m.setCursor].edit(&m.cfg, delta)
			if m.cfg.SpinnerStyle != before {
				// recria o spinner com o novo estilo e reinicia o tick dele
				m.spin = newSpinner(m.cfg.SpinnerStyle)
				return m, m.spin.Tick
			}
		}
		return m, nil

	default: // viewNormal
		switch k {
		case "q", "esc":
			return m, tea.Quit
		case "?":
			m.mode = viewHelp
		case "v":
			m.cfg.Layout = nextLayout(m.cfg.Layout)
			m.scroll = 0
			_ = saveConfig(m.cfg)
			m.setNotice("layout: " + layoutLabel(m.cfg.Layout))
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			if m.scroll < m.maxScroll(m.effWidth()) {
				m.scroll++
			}
		case "s":
			m.mode = viewSettings
			m.setCursor = 0
		case "m":
			m.cfg.Sound = !m.cfg.Sound
			_ = saveConfig(m.cfg)
			m.setNotice(ternary(m.cfg.Sound, "som ligado", "som mutado"))
		case "f":
			if m.filter == filterAll {
				m.filter = filterAttention
				m.setNotice("filtro: só precisa de você")
			} else {
				m.filter = filterAll
				m.setNotice("filtro: tudo")
			}
			m.cfg.OnlyAttention = m.filter == filterAttention
			_ = saveConfig(m.cfg)
			m.reload()
		case "r":
			m.reload()
			m.setNotice("recarregado")
		case "c":
			m.clearedAt = m.now
			m.reload()
			m.setNotice("sessões paradas limpas")
		}
		return m, nil
	}
}

func ternary(b bool, a, c string) string {
	if b {
		return a
	}
	return c
}

// effWidth devolve a largura efetiva usada no render (com o mesmo piso do View).
func (m model) effWidth() int {
	if m.width < 20 {
		return 48
	}
	return m.width
}

// layoutDims devolve quantas linhas o topo ocupa (header+quota+régua) e quantas
// sobram pro corpo (entre o topo e o rodapé de 2 linhas).
func (m model) layoutDims(w int) (topLines, bodyH int) {
	topLines = 1 + len(m.quotaLines(w)) + 1 // header + linhas de quota + régua
	bodyH = m.height - topLines - 2         // 2 = régua + rodapé
	if bodyH < 1 {
		bodyH = 1
	}
	return
}

// animateQuota move as frações exibidas rumo aos valores reais (deslize) e
// troca o gradiente quando a quota cruza uma faixa de limiar.
func (m *model) animateQuota() {
	step := func(bar *float64, band *int, prog *progress.Model, p float64) {
		if b := bandOf(p); b != *band {
			*band = b
			*prog = newQuotaBar(b)
		}
		*bar = ease(*bar, quotaFrac(p))
	}
	step(&m.bar5, &m.band5, &m.prog5, m.quota.FiveHour)
	step(&m.bar7, &m.band7, &m.prog7, m.quota.SevenDay)
}

// reload relê o log, deriva o estado e dispara o alarme p/ novas atenções.
func (m *model) reload() {
	events, _ := readEvents()
	sessions := deriveSessions(events)
	m.loadUsage()

	// quota da conta (vinda do statusline via `mon quota`)
	m.quota = readQuota()
	if m.quota.Blocked {
		if !m.quotaRung {
			m.quotaRung = true
			m.alarm()
		}
	} else {
		m.quotaRung = false
	}

	// alarme: alguém NOVO entrou em "attention"?
	active := map[string]bool{}
	newAttention := false
	for id, s := range sessions {
		if s.Kind == KindAttention {
			active[id] = true
			if !m.rung[id] {
				m.rung[id] = true
				newAttention = true
			}
		}
	}
	// limpa quem saiu de attention (pode alarmar de novo no futuro)
	for id := range m.rung {
		if !active[id] {
			delete(m.rung, id)
		}
	}
	if newAttention {
		m.alarm()
	}

	// filtra sessões velhas/limpas/filtradas e ordena por prioridade
	stale := time.Duration(m.cfg.StaleMinutes) * time.Minute
	var list []*Session
	for _, s := range sessions {
		// Só "working" fica imune ao tempo — uma tarefa longa pode ficar
		// silenciosa (sem novos hooks) e não deve sumir. Todo o resto envelhece,
		// INCLUSIVE "attention": quando você resolve a permissão fora da nossa
		// visão (id da sessão muda, Claude fecha sem Stop, ferramenta longa…),
		// nenhum hook limpa o estado — então ela some depois de StaleMinutes.
		// O alarme já tocou quando ela apareceu, então nada se perde.
		agesOut := s.Kind != KindWorking
		if agesOut && m.now.Sub(s.LastSeen) > stale {
			continue // muito antiga
		}
		if agesOut && !m.clearedAt.IsZero() && !s.LastSeen.After(m.clearedAt) {
			continue // limpa manual (tecla 'c') — inclui attention presa
		}
		if m.filter == filterAttention && s.Kind != KindAttention {
			continue
		}
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if pr(list[i].Kind) != pr(list[j].Kind) {
			return pr(list[i].Kind) < pr(list[j].Kind)
		}
		return list[i].LastSeen.After(list[j].LastSeen)
	})
	m.sessions = list

	// mantém a rolagem dentro do intervalo válido (contagem/tamanho mudaram)
	if ms := m.maxScroll(m.effWidth()); m.scroll > ms {
		m.scroll = ms
	}
}

// pr define a ordem de exibição (menor = mais no topo).
func pr(k Kind) int {
	switch k {
	case KindAttention:
		return 0
	case KindWorking:
		return 1
	case KindDone:
		return 2
	default:
		return 3
	}
}

// alarm: BEL (cool-retro-terminal costuma piscar) + som no macOS.
func (m *model) alarm() {
	fmt.Fprint(os.Stderr, "\a")
	if !m.cfg.Sound {
		return
	}
	if _, err := os.Stat(m.cfg.SoundPath); err == nil {
		_ = exec.Command("afplay", m.cfg.SoundPath).Start() // fire-and-forget
	}
}

// ---- view -----------------------------------------------------------------

func (m model) View() string {
	w := m.width
	if w < 20 {
		w = 48
	}

	switch m.mode {
	case viewHelp:
		return m.renderHelp(w)
	case viewSettings:
		return m.renderSettings(w)
	}

	var b strings.Builder

	// cabeçalho: título à esquerda, relógio à direita (com 1 col de respiro)
	title := stTitle.Render(" CLAUDE CODE MONITOR")
	clockFmt := "15:04:05"
	if !m.cfg.Clock24h {
		clockFmt = "3:04:05 PM"
	}
	clock := stDim.Render(m.now.Format(clockFmt) + " ")
	b.WriteString(spread(title, clock, w))
	b.WriteByte('\n')

	// quota da conta FICA ACIMA da linha separadora (junto do cabeçalho)
	for _, ql := range m.quotaLines(w) {
		b.WriteString(ql)
		b.WriteByte('\n')
	}

	b.WriteString(stRule.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')

	// corpo: lista, grid ou stats — sempre com a altura exata disponível
	_, bodyH := m.layoutDims(w)
	for _, ln := range m.renderBody(w, bodyH) {
		b.WriteString(ln)
		b.WriteByte('\n')
	}

	// rodapé
	b.WriteString(stRule.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')
	b.WriteString(m.footer(w))
	return b.String()
}

// blink alterna a cada ~450ms (3 frames de 150ms) — visível sem estressar.
// Se o piscar está desligado, mantém sempre o estado "aceso" (barra sólida).
func (m model) blink() bool {
	if !m.cfg.Blink {
		return true
	}
	return (m.frame/3)%2 == 0
}

func (m model) renderRow(s *Session, w int) string {
	// texto do subtítulo (linha 2): título da sessão ou fallback #hash
	subtitle := s.Title
	if subtitle == "" {
		subtitle = "#" + shortID(s.ID)
	}

	// "precisa de você": linha inteira PISCA (barra vermelha) — sem depender
	// de cor só, e sem trocar a largura de nenhum char (fim do chacoalho).
	if s.Kind == KindAttention {
		return m.renderAlarmRow(s, subtitle, w)
	}

	var icon, label string
	var style lipgloss.Style
	switch s.Kind {
	case KindWorking:
		icon = m.spin.View()
		label, style = "trabalhando", stWorking
	case KindDone:
		icon, label, style = "✓", "pronto", stDone
	case KindStart:
		icon, label, style = "○", "iniciando", stIdle
	default:
		icon, label, style = "·", "ocioso", stIdle
	}

	// linha 1: [ícone] projeto (cor da sessão, negrito) ..... status (à direita)
	// status termina em w-1 (1 char de margem), igual ao rodapé e à quota.
	projStyle := stProject.Foreground(sessionColor(s.ID))
	projMax := w - 3 - lipgloss.Width(label) - 2
	if projMax < 6 {
		projMax = 6
	}
	left := " " + style.Render(icon) + " " + projStyle.Render(trunc(s.Project, projMax))
	line1 := spread(left, style.Render(label)+" ", w)
	line2 := "   " + stSubtitle.Render(trunc(subtitle, w-4))
	return line1 + "\n" + line2
}

// sessionPalette: cores estáveis e distintas p/ identificar cada sessão.
// Evita vermelho (reservado ao alarme).
var sessionPalette = []lipgloss.Color{
	"39", "213", "214", "84", "141", "208", "45", "199", "156", "111",
}

// sessionColor deriva uma cor estável do id da sessão (hash simples).
func sessionColor(id string) lipgloss.Color {
	var h uint32 = 2166136261
	for i := 0; i < len(id); i++ { // FNV-1a
		h ^= uint32(id[i])
		h *= 16777619
	}
	return sessionPalette[h%uint32(len(sessionPalette))]
}

// renderAlarmRow desenha as duas linhas como texto puro do MESMO tamanho e
// pisca a linha toda entre "barra vermelha cheia" e "texto vermelho".
func (m model) renderAlarmRow(s *Session, subtitle string, w int) string {
	label := "precisa de você"
	icon := "▲" // largura 1, estável (nada de troca de char)

	projMax := w - 3 - lipgloss.Width(label) - 2
	if projMax < 6 {
		projMax = 6
	}
	left := " " + icon + " " + trunc(s.Project, projMax)
	line1 := padRight(spread(left, label+" ", w), w)
	line2 := padRight("   "+trunc(subtitle, w-4), w)

	var st lipgloss.Style
	if m.blink() {
		st = stAlarmOn // texto claro sobre fundo vermelho (barra cheia)
	} else {
		st = stAlarmOff // vermelho sobre fundo padrão
	}
	return st.Render(line1) + "\n" + st.Render(line2)
}

// shortID devolve os primeiros 8 caracteres do id (antes do primeiro '-').
func shortID(id string) string {
	if id == "" {
		return "?"
	}
	r := []rune(id)
	if len(r) > 8 {
		r = r[:8]
	}
	return string(r)
}

// quotaLines desenha a quota da conta em DUAS linhas (uma por janela), com
// barra, % e horário de reset — pra deixar claro consumo e quando reinicia.
// "5h" = sessão atual (janela de 5h); "7d" = limite semanal.
func (m model) quotaLines(w int) []string {
	q := m.quota
	if !m.cfg.ShowQuota || !q.Present {
		return nil
	}

	if q.Blocked {
		msg := " ▲ LIMITE DO PLANO"
		if !q.Reset.IsZero() {
			msg += " — reseta " + resetLabel(q.Reset, m.now)
		}
		st := stAlarmOff
		if m.blink() {
			st = stAlarmOn
		}
		return []string{st.Render(padRight(msg, w))}
	}

	// alinha as duas linhas usando o mesmo tamanho de bloco à direita
	r5 := quotaRight(q.FiveHour, q.Reset, m.now)
	r7 := quotaRight(q.SevenDay, q.SevenReset, m.now)
	rw := lipgloss.Width(r5)
	if x := lipgloss.Width(r7); x > rw {
		rw = x
	}
	return []string{
		m.quotaBarLine("5h", r5, rw, w, m.prog5, m.bar5),
		m.quotaBarLine("7d", r7, rw, w, m.prog7, m.bar7),
	}
}

// quotaRight monta o bloco direito: "25%  ↻ 1h05".
func quotaRight(p float64, reset, now time.Time) string {
	if p < 0 {
		return "sem dados"
	}
	s := fmt.Sprintf("%3.0f%%", p)
	if !reset.IsZero() {
		s += "  · " + resetLabel(reset, now)
	}
	return s
}

// quotaBarLine: " 5h ███████░░░░░░   25%  · 1h05" (largura exata w). A barra é
// o progress do bubbles (gradiente da faixa) renderizado na fração animada.
func (m model) quotaBarLine(label, right string, rw, w int, prog progress.Model, frac float64) string {
	left := " " + label + " "
	right = strings.Repeat(" ", rw-lipgloss.Width(right)) + right + " " // margem
	barW := w - lipgloss.Width(left) - lipgloss.Width(right)
	if barW < 4 {
		barW = 4
	}
	prog.Width = barW // cópia por valor: seguro ajustar aqui
	return stDim.Render(left) + prog.ViewAs(frac) + stDim.Render(right)
}

// resetLabel: "45m" / "1h05" (se <24h) ou "dom 15h" (se mais longe).
func resetLabel(reset, now time.Time) string {
	d := reset.Sub(now)
	switch {
	case d <= 0:
		return "agora"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
	default:
		wd := []string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}[reset.Local().Weekday()]
		return wd + " " + reset.Local().Format("15:04")
	}
}

func (m model) footer(w int) string {
	// mensagem transitória (ex.: "config salva") tem prioridade à esquerda
	var left string
	if m.notice != "" {
		left = stDone.Render(" " + m.notice)
	} else {
		att := 0
		for _, s := range m.sessions {
			if s.Kind == KindAttention {
				att++
			}
		}
		if att > 0 {
			left = stAtten.Render(fmt.Sprintf(" ⚠ %d precisa de você", att))
		} else if m.filter == filterAttention {
			left = stDim.Render(" filtro: só precisa de você")
		} else {
			left = stDim.Render(" ? ajuda")
		}
	}
	noun := "sessões"
	if len(m.sessions) == 1 {
		noun = "sessão"
	}
	right := stDim.Render(fmt.Sprintf("%d %s ", len(m.sessions), noun))
	return spread(left, right, w)
}

// ---- settings & ajuda -----------------------------------------------------

type settingItem struct {
	label string
	value func(c Config) string
	edit  func(c *Config, delta int)
}

func onOff(b bool) string {
	if b {
		return "ligado"
	}
	return "desligado"
}

var settingItems = []settingItem{
	{"Layout", func(c Config) string { return layoutLabel(c.Layout) }, func(c *Config, d int) {
		// cicla nos dois sentidos: lista ↔ grid ↔ auto
		if d < 0 {
			c.Layout = nextLayout(nextLayout(c.Layout))
		} else {
			c.Layout = nextLayout(c.Layout)
		}
	}},
	{"Colunas do grid", func(c Config) string { return fmt.Sprintf("%d", c.GridCols) }, func(c *Config, d int) {
		c.GridCols += d
		if c.GridCols < 1 {
			c.GridCols = 1
		}
		if c.GridCols > maxGridCols {
			c.GridCols = maxGridCols
		}
	}},
	{"Estilo do spinner", func(c Config) string { return c.SpinnerStyle }, func(c *Config, d int) {
		c.SpinnerStyle = cycleSpinner(c.SpinnerStyle, d)
	}},
	{"Som do alarme", func(c Config) string { return onOff(c.Sound) }, func(c *Config, d int) { c.Sound = !c.Sound }},
	{"Piscar alarme", func(c Config) string { return onOff(c.Blink) }, func(c *Config, d int) { c.Blink = !c.Blink }},
	{"Mostrar quota", func(c Config) string { return onOff(c.ShowQuota) }, func(c *Config, d int) { c.ShowQuota = !c.ShowQuota }},
	{"Relógio 24h", func(c Config) string { return onOff(c.Clock24h) }, func(c *Config, d int) { c.Clock24h = !c.Clock24h }},
	{"Sumir ociosa (min)", func(c Config) string { return fmt.Sprintf("%d", c.StaleMinutes) }, func(c *Config, d int) {
		c.StaleMinutes += d * 5
		if c.StaleMinutes < 5 {
			c.StaleMinutes = 5
		}
		if c.StaleMinutes > 240 {
			c.StaleMinutes = 240
		}
	}},
}

// overlay renderiza uma tela cheia (título, corpo, rodapé) com a altura exata.
func (m model) overlay(title string, rows []string, hint string, w int) string {
	var b strings.Builder
	b.WriteString(stTitle.Render(title))
	b.WriteByte('\n')
	b.WriteString(stRule.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')
	used := 2
	for _, r := range rows {
		b.WriteString(r)
		b.WriteByte('\n')
		used++
	}
	for used < m.height-2 {
		b.WriteByte('\n')
		used++
	}
	b.WriteString(stRule.Render(strings.Repeat("─", w)))
	b.WriteByte('\n')
	b.WriteString(stDim.Render(hint))
	return b.String()
}

func (m model) renderSettings(w int) string {
	var rows []string
	for i, it := range settingItems {
		marker, lab := "  ", stDim.Render(it.label)
		if i == m.setCursor {
			marker = stAtten.Render(" ›")
			lab = stProject.Render(it.label)
		}
		val := stWorking.Render(it.value(m.cfg))
		rows = append(rows, spread(marker+" "+lab, val+"  ", w))
	}
	rows = append(rows,
		"",
		stDim.Render(" som: "+filepath.Base(m.cfg.SoundPath)+" (via config.json)"),
	)
	return m.overlay("⚙ CONFIGURAÇÕES", rows, " ↑↓ move · ←→/espaço muda · s/esc salva", w)
}

func (m model) renderHelp(w int) string {
	keys := [][2]string{
		{"?", "esta ajuda"},
		{"v", "layout (lista / grid / auto)"},
		{"↑↓", "rolar a lista/grid"},
		{"s", "configurações"},
		{"m", "mutar / desmutar som"},
		{"f", "filtrar (tudo / precisa de você)"},
		{"r", "recarregar agora"},
		{"c", "limpar paradas (inclui presas em 'precisa de você')"},
		{"q", "sair"},
	}
	var rows []string
	for _, k := range keys {
		rows = append(rows, " "+stProject.Render(k[0])+"  "+stDim.Render(k[1]))
	}
	filt := "tudo"
	if m.filter == filterAttention {
		filt = "só atenção"
	}
	rows = append(rows,
		"",
		stDim.Render(fmt.Sprintf(" som: %s   filtro: %s", onOff(m.cfg.Sound), filt)),
	)
	return m.overlay("⌨  ATALHOS", rows, " qualquer tecla fecha", w)
}

// ---- helpers de layout ----------------------------------------------------

// spread coloca left à esquerda e right à direita, preenchendo w colunas.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// padRight completa a string com espaços até ocupar w colunas.
func padRight(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

func center(s string, w int) string {
	pad := (w - lipgloss.Width(s)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func runTUI() int {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	return 0
}
