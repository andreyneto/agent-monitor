package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/mattn/go-runewidth"
)

func TestGroupInt(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 1000: "1.000", 43026: "43.026", 10465: "10.465", -1234: "-1.234"}
	for in, want := range cases {
		if got := groupInt(in); got != want {
			t.Errorf("groupInt(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestAbbrevTokens(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 162341: "162K", 878754: "879K", 21298659: "21.3M"}
	for in, want := range cases {
		if got := abbrevTokens(in); got != want {
			t.Errorf("abbrevTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestShortModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-8":           "opus 4.8",
		"claude-haiku-4-5-20251001": "haiku 4.5",
		"claude-opus-4-7":           "opus 4.7",
		"claude-sonnet-5":           "sonnet 5",
	}
	for in, want := range cases {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSpinnerWidthStable garante que todo frame do spinner tenha largura 1
// (inclusive sob East-Asian) — glifo ambíguo/emoji faz a borda do card oscilar
// no grid. O spinner usado é o MiniDot (braille), largura 1 estável.
func TestSpinnerWidthStable(t *testing.T) {
	ea := runewidth.NewCondition()
	ea.EastAsianWidth = true
	for i, f := range spinner.MiniDot.Frames {
		if r := []rune(f); len(r) != 1 {
			t.Errorf("frame %d (%q) não é 1 rune", i, f)
		}
		if w := ea.StringWidth(f); w != 1 {
			t.Errorf("frame %d (%q) tem largura %d (EastAsian), esperado 1", i, f, w)
		}
	}
}
