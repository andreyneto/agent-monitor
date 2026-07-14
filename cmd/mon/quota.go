package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func quotaPath() string { return filepath.Join(monDir(), "quota.json") }

// QuotaFile é o que gravamos (overwrite) a partir do statusline.
// Guardamos o rate_limits verbatim pra não depender de adivinhar o schema.
type QuotaFile struct {
	Time time.Time       `json:"time"`
	Raw  json.RawMessage `json:"raw"`
}

// runQuota lê o JSON do statusline no stdin e salva rate_limits em quota.json.
// Roda a cada refresh do statusline; sobrescreve (não faz append) → sem bloat.
func runQuota(args []string) int {
	raw, _ := io.ReadAll(os.Stdin)
	var in struct {
		RateLimits json.RawMessage `json:"rate_limits"`
	}
	if err := json.Unmarshal(raw, &in); err != nil || len(in.RateLimits) == 0 {
		return 0 // sem rate_limits (não-assinante / antes da 1ª resposta): ignora
	}
	if err := os.MkdirAll(monDir(), 0o755); err != nil {
		return 0
	}
	out, err := json.Marshal(QuotaFile{Time: time.Now(), Raw: in.RateLimits})
	if err != nil {
		return 0
	}
	tmp := quotaPath() + ".tmp"
	if os.WriteFile(tmp, out, 0o644) == nil {
		_ = os.Rename(tmp, quotaPath()) // troca atômica
	}
	return 0
}

// Quota é a visão normalizada que a TUI consome.
type Quota struct {
	Present    bool
	Age        time.Duration
	FiveHour   float64   // 0..100 (-1 = desconhecido)
	SevenDay   float64   // 0..100 (-1 = desconhecido)
	Reset      time.Time // reset da janela de 5h
	SevenReset time.Time // reset da janela semanal
	Blocked    bool      // limite atingido / sem créditos
	Warning    string    // texto de aviso, quando houver
}

// readQuota lê e normaliza o quota.json. É tolerante ao schema: procura
// utilization/resets em vários formatos porque o layout exato do rate_limits
// pode variar entre versões do Claude Code.
func readQuota() Quota {
	q := Quota{FiveHour: -1, SevenDay: -1}
	b, err := os.ReadFile(quotaPath())
	if err != nil {
		return q
	}
	var qf QuotaFile
	if json.Unmarshal(b, &qf) != nil {
		return q
	}
	var m map[string]any
	if json.Unmarshal(qf.Raw, &m) != nil {
		return q
	}
	q.Present = true
	q.Age = time.Since(qf.Time)
	q.FiveHour = pct(dig(m, "five_hour"))
	q.SevenDay = pct(dig(m, "seven_day"))
	q.Reset = resetTime(m["five_hour"])
	q.SevenReset = resetTime(m["seven_day"])

	// estado geral: procura por sinais de bloqueio/aviso em qualquer string
	blob := strings.ToLower(string(qf.Raw))
	if strings.Contains(blob, "rejected") || strings.Contains(blob, "out_of_credits") ||
		strings.Contains(blob, "zero_credit") {
		q.Blocked = true
	}
	q.Warning = firstWarning(m)
	return q
}

// dig acha um sub-objeto por chave (ou o próprio valor se já for numérico).
func dig(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	return nil
}

// pct extrai um percentual (0..100) de um valor que pode ser número (0..1 ou
// 0..100) ou um objeto com utilization/used_pct/percent.
func pct(v any) float64 {
	switch t := v.(type) {
	case float64:
		return scale(t)
	case map[string]any:
		// chaves que JÁ vêm em 0..100: usa direto, sem a heurística de fração —
		// senão used_percentage:1 (1% usado) viraria 100%.
		for _, k := range []string{"used_percentage", "used_pct", "percent"} {
			if n, ok := t[k].(float64); ok {
				return clampPct(n)
			}
		}
		// chaves ambíguas: podem vir como fração 0..1 (ex.: utilization:0.62)
		for _, k := range []string{"utilization", "used", "usage"} {
			if n, ok := t[k].(float64); ok {
				return scale(n)
			}
		}
	}
	return -1
}

// scale normaliza um número ambíguo: valores ≤ 1 são tratados como fração 0..1.
func scale(n float64) float64 {
	if n <= 1.0 { // veio como fração 0..1
		n *= 100
	}
	return clampPct(n)
}

// clampPct prende o percentual em 0..100 (negativo = desconhecido).
func clampPct(n float64) float64 {
	if n < 0 {
		return -1
	}
	if n > 100 {
		return 100
	}
	return n
}

func resetTime(v any) time.Time {
	m, ok := v.(map[string]any)
	if !ok {
		return time.Time{}
	}
	for _, k := range []string{"resets_at", "reset_at", "resets", "reset"} {
		switch t := m[k].(type) {
		case string:
			if ts, err := time.Parse(time.RFC3339, t); err == nil {
				return ts
			}
		case float64:
			return time.Unix(int64(t), 0)
		}
	}
	return time.Time{}
}

func firstWarning(m map[string]any) string {
	for _, k := range []string{"warning", "message", "status"} {
		if s, ok := m[k].(string); ok && s != "" && s != "allowed" {
			return s
		}
	}
	return ""
}
