package main

import (
	"fmt"
	"os"
)

const usage = `mon — monitor das suas sessões de IA

uso:
  mon               abre o painel TUI (rode isso no monitorzinho)
  mon emit          lê um evento (JSON no stdin) e grava — usado pelos hooks
  mon install       instala os hooks no ~/.claude/settings.json (com backup)
  mon test          injeta sessões de exemplo pra ver o painel funcionando
  mon help          esta ajuda

ambiente:
  MON_DIR     onde ficam os eventos (default: ~/.claude/monitor)
  MON_SOUND   som do alarme (default: /System/Library/Sounds/Glass.aiff)
  MON_SILENT  se setado, desliga o som (BEL continua)
`

func main() {
	if len(os.Args) < 2 {
		os.Exit(runTUI())
	}
	switch os.Args[1] {
	case "emit":
		os.Exit(runEmit(os.Args[2:]))
	case "quota":
		os.Exit(runQuota(os.Args[2:]))
	case "install":
		os.Exit(runInstall(os.Args[2:]))
	case "test":
		os.Exit(runTest(os.Args[2:]))
	case "tui", "run":
		os.Exit(runTUI())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintln(os.Stderr, "comando desconhecido:", os.Args[1])
		fmt.Print(usage)
		os.Exit(2)
	}
}
