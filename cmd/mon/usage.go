package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// statsCachePath aponta pro cache de uso do próprio Claude Code (o mesmo que
// alimenta o /usage). Fica em ~/.claude/stats-cache.json; MON_STATS sobrescreve.
func statsCachePath() string {
	if p := os.Getenv("MON_STATS"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "stats-cache.json")
}

// Usage é a visão normalizada do stats-cache.json que a tela vazia consome.
type Usage struct {
	Present       bool
	TotalSessions int
	TotalTokens   int            // input+output somados (não conta cache)
	Favorite      string         // modelo mais usado, já encurtado ("opus 4.8")
	LongestMs     int64          // duração da sessão mais longa (ms)
	FirstSession  time.Time      // primeira sessão registrada
	LastComputed  time.Time      // até quando o Claude computou o cache (/usage)
	Daily         map[string]int // data (YYYY-MM-DD) → mensagens no dia
}

// rawStats espelha só o que a gente lê do stats-cache.json.
type rawStats struct {
	DailyActivity []struct {
		Date         string `json:"date"`
		MessageCount int    `json:"messageCount"`
	} `json:"dailyActivity"`
	ModelUsage map[string]struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	} `json:"modelUsage"`
	TotalSessions    int    `json:"totalSessions"`
	LastComputedDate string `json:"lastComputedDate"`
	FirstSessionDate string `json:"firstSessionDate"`
	LongestSession   struct {
		Duration int64 `json:"duration"` // milissegundos
	} `json:"longestSession"`
}

// loadUsage lê o stats-cache.json e preenche m.usage, com cache por mtime (a
// tela vazia re-renderiza a cada tick e o arquivo raramente muda).
func (m *model) loadUsage() {
	fi, err := os.Stat(statsCachePath())
	if err != nil {
		m.usage = Usage{}
		m.usageMtime = time.Time{}
		return
	}
	if m.usage.Present && fi.ModTime().Equal(m.usageMtime) {
		return // sem mudança → mantém o parse anterior
	}
	m.usageMtime = fi.ModTime()
	m.usage = parseUsage()
}

func parseUsage() Usage {
	u := Usage{Daily: map[string]int{}}
	b, err := os.ReadFile(statsCachePath())
	if err != nil {
		return u
	}
	var r rawStats
	if json.Unmarshal(b, &r) != nil {
		return u
	}
	u.Present = true
	u.TotalSessions = r.TotalSessions
	u.LongestMs = r.LongestSession.Duration
	for _, d := range r.DailyActivity {
		u.Daily[d.Date] = d.MessageCount
	}
	if t, err := time.Parse(time.RFC3339, r.FirstSessionDate); err == nil {
		u.FirstSession = t
	}
	// data-só (sem fuso): parseia em local pra não escorregar 1 dia ao exibir
	if t, err := time.ParseInLocation("2006-01-02", r.LastComputedDate, time.Local); err == nil {
		u.LastComputed = t
	}
	// modelo favorito + total de tokens (input+output)
	bestTok := -1
	for name, mu := range r.ModelUsage {
		io := mu.InputTokens + mu.OutputTokens
		u.TotalTokens += io
		if io > bestTok {
			bestTok = io
			u.Favorite = shortModel(name)
		}
	}
	return u
}

// derived são as estatísticas que dependem de "hoje" (span, dia mais ativo,
// sequências), calculadas a partir do Daily.
type derived struct {
	activeDays int
	spanDays   int
	longest    int // maior sequência de dias consecutivos com atividade
	current    int // sequência atual (terminando hoje/ontem)
	mostDate   time.Time
	mostCount  int
}

func (u Usage) derive(now time.Time) derived {
	var d derived
	active := map[string]bool{}
	var dates []time.Time
	for ds, n := range u.Daily {
		if n <= 0 {
			continue
		}
		active[ds] = true
		if n > d.mostCount {
			if t, err := time.Parse("2006-01-02", ds); err == nil {
				d.mostCount = n
				d.mostDate = t
			}
		}
		if t, err := time.Parse("2006-01-02", ds); err == nil {
			dates = append(dates, t)
		}
	}
	d.activeDays = len(active)

	today := dateOnly(now)
	if !u.FirstSession.IsZero() {
		d.spanDays = int(today.Sub(dateOnly(u.FirstSession)).Hours()/24) + 1
	}

	// maior sequência: varre as datas ordenadas contando dias consecutivos
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	run := 0
	var prev time.Time
	for _, t := range dates {
		if !prev.IsZero() && t.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > d.longest {
			d.longest = run
		}
		prev = t
	}

	// sequência atual: anda pra trás a partir de hoje (ou ontem, se hoje vazio)
	cur := today
	if !active[cur.Format("2006-01-02")] {
		cur = cur.AddDate(0, 0, -1)
	}
	for active[cur.Format("2006-01-02")] {
		d.current++
		cur = cur.AddDate(0, 0, -1)
	}
	return d
}

// staleNote avisa que o overview sai do cache do Claude Code (o mesmo do
// /usage), que só recomputa quando você abre o /usage. Devolve "" se o cache
// já é de hoje. Assim os números batem exatamente com o /usage, e a nota deixa
// explícito quando estão velhos.
func (u Usage) staleNote(now time.Time) string {
	if u.LastComputed.IsZero() {
		return ""
	}
	if !dateOnly(u.LastComputed).Before(dateOnly(now)) {
		return "" // computado hoje: fresco
	}
	return "dados até " + dateShort(u.LastComputed) + " · abra /usage"
}

func dateOnly(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// shortModel encurta "claude-opus-4-8" → "opus 4.8" e tira sufixo de data.
func shortModel(name string) string {
	s := strings.TrimPrefix(name, "claude-")
	parts := strings.Split(s, "-")
	// descarta um sufixo que seja data compactada (ex.: 20251001)
	if n := len(parts); n > 0 && len(parts[n-1]) >= 6 && allDigits(parts[n-1]) {
		parts = parts[:n-1]
	}
	if len(parts) == 0 {
		return name
	}
	family := parts[0]
	if len(parts) == 1 {
		return family
	}
	return family + " " + strings.Join(parts[1:], ".")
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// capWord deixa a 1ª letra maiúscula: "opus 4.8" → "Opus 4.8".
func capWord(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 32
	}
	return string(r)
}

// groupInt formata um inteiro com "." de milhar (pt-BR): 43026 → "43.026".
func groupInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// abbrevTokens encurta contagens grandes: 28188262 → "28.2M", 162341 → "162K".
func abbrevTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// durLabel formata uma duração em ms de forma curta: "49d 22h", "3h 05m", "12m".
func durLabel(ms int64) string {
	sec := ms / 1000
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// dateShort formata uma data curta em pt-BR: "1 jul".
func dateShort(t time.Time) string {
	months := []string{"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}
	t = t.Local()
	return fmt.Sprintf("%d %s", t.Day(), months[int(t.Month())-1])
}
