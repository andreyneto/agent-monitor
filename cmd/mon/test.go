package main

import (
	"fmt"
	"time"
)

// runTest injeta sessões de exemplo no log pra visualizar o painel.
func runTest(args []string) int {
	now := time.Now()
	samples := []Event{
		{Session: "s1", Project: "bp-athena", Cwd: "/x/athena", Kind: KindAttention, Message: "permissão", Time: now},
		{Session: "s2", Project: "fmoney", Cwd: "/x/fmoney", Kind: KindWorking, Time: now.Add(-64 * time.Second)},
		{Session: "s3", Project: "lumina", Cwd: "/x/lumina", Kind: KindDone, Time: now.Add(-3 * time.Minute)},
		{Session: "s4", Project: "copa", Cwd: "/x/copa", Kind: KindWorking, Time: now.Add(-10 * time.Second)},
		{Session: "s5", Project: "monitor", Cwd: "/x/monitor", Kind: KindStart, Time: now.Add(-30 * time.Second)},
		{Session: "s6", Project: "lullari", Cwd: "/x/lullari", Kind: KindBackground, BgTasks: []string{"Boot Android Pixel_10_Pro emulator", "iOS sim build"}, Time: now.Add(-90 * time.Second)},
	}
	for _, e := range samples {
		e.Source = "test"
		if err := appendEvent(e); err != nil {
			fmt.Println("erro:", err)
			return 1
		}
	}
	fmt.Printf("✓ %d sessões de exemplo gravadas em %s\n", len(samples), eventsPath())
	fmt.Println("agora rode:  mon")
	return 0
}
