package main

import (
	"math"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// ---- spinner (bubbles) -----------------------------------------------------

const spinnerDefault = "minidot"

// spinnerStyles são os estilos oferecidos — todos com frames de largura
// UNIFORME (não oscilam a borda do card). Evitamos os emoji (Globe/Moon/Monkey)
// e os de largura variável.
var spinnerStyles = []string{"minidot", "pulse", "line", "jump", "dot"}

func validSpinner(name string) bool {
	for _, s := range spinnerStyles {
		if s == name {
			return true
		}
	}
	return false
}

// cycleSpinner devolve o próximo (delta>0) ou anterior estilo da lista.
func cycleSpinner(cur string, delta int) string {
	i := 0
	for j, s := range spinnerStyles {
		if s == cur {
			i = j
			break
		}
	}
	n := len(spinnerStyles)
	return spinnerStyles[((i+delta)%n+n)%n]
}

func spinnerFor(name string) spinner.Spinner {
	switch name {
	case "pulse":
		return spinner.Pulse
	case "line":
		return spinner.Line
	case "jump":
		return spinner.Jump
	case "dot":
		return spinner.Dot
	default:
		return spinner.MiniDot
	}
}

// newSpinner cria o spinner sem estilo próprio (View() devolve só o glifo; a cor
// é aplicada por quem renderiza, com stWorking).
func newSpinner(name string) spinner.Model {
	s := spinner.New(spinner.WithSpinner(spinnerFor(name)))
	s.Style = lipgloss.NewStyle()
	return s
}

// ---- barra de quota (progress do bubbles, animada) -------------------------

// quotaGrad devolve o gradiente (hex A→B) de cada faixa de limiar. Mantém o
// padrão de cores (laranja < 70%, âmbar 70–90%, vermelho ≥ 90%) e adiciona
// profundidade: vai do tom escuro (esquerda) ao tom vivo da faixa (direita).
func quotaGrad(band int) (a, b string) {
	switch band {
	case 2:
		return "#800000", "#ff3030" // vermelho (≥90%)
	case 1:
		return "#6b4a00", "#ffaf00" // âmbar (70–90%)
	default:
		return "#6b3600", "#ff8700" // laranja (<70%)
	}
}

// bandOf mapeia o percentual (0..100) na faixa de limiar 0/1/2.
func bandOf(p float64) int {
	switch {
	case p >= 90:
		return 2
	case p >= 70:
		return 1
	default:
		return 0
	}
}

// newQuotaBar cria uma barra com o gradiente da faixa, sem % (a gente desenha o
// número por fora) e com o trilho dim (░) igual ao resto do painel.
func newQuotaBar(band int) progress.Model {
	a, b := quotaGrad(band)
	p := progress.New(
		progress.WithScaledGradient(a, b), // gradiente sempre preenche a parte cheia
		progress.WithoutPercentage(),
		// usa o MESMO profile que o lipgloss (que já renderiza cor no app),
		// em vez da autodetecção própria do progress.
		progress.WithColorProfile(lipgloss.DefaultRenderer().ColorProfile()),
	)
	p.EmptyColor = "244" // trilho ░ (mesmo tom do stDim)
	return p
}

// quotaFrac normaliza o percentual (0..100, -1 = sem dados) em 0..1.
func quotaFrac(p float64) float64 {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 1
	default:
		return p / 100
	}
}

// ease aproxima cur de target com suavização exponencial (efeito de deslize).
func ease(cur, target float64) float64 {
	if d := target - cur; math.Abs(d) < 0.004 {
		return target
	} else {
		return cur + d*0.28
	}
}
