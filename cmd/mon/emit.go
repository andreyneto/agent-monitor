package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// hookPayload é o JSON que o Claude Code manda no stdin dos hooks.
// Só declaramos os campos que interessam; o resto é ignorado.
type hookPayload struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	Message        string `json:"message"`
	TranscriptPath string `json:"transcript_path"`
	// nome/entrada da ferramenta (PostToolUse/PreToolUse)
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	// tarefas em background rodando (Stop/SubagentStop carregam essa lista)
	BackgroundTasks []bgTask `json:"background_tasks"`
	// permite empurrar evento manualmente com kind/título explícito
	Kind    string `json:"kind"`
	Project string `json:"project"`
	Source  string `json:"source"`
	Title   string `json:"title"`
}

// bgTask espelha uma tarefa em background do payload (Stop/SubagentStop):
// shell rodando, subagente, etc. Só usamos status e descrição.
type bgTask struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

// runningBgTasks devolve as descrições das tarefas em background que ainda
// estão rodando (status "running").
func runningBgTasks(tasks []bgTask) []string {
	var out []string
	for _, t := range tasks {
		if t.Status != "running" {
			continue
		}
		d := t.Description
		if d == "" {
			d = "tarefa em background"
		}
		out = append(out, d)
	}
	return out
}

// mapHookKind traduz o nome do evento do Claude para o nosso Kind.
func mapHookKind(hookEvent string) Kind {
	switch hookEvent {
	case "SessionStart":
		return KindStart
	case "UserPromptSubmit":
		return KindWorking
	case "Notification":
		return KindAttention
	case "PostToolUse":
		return KindWorking
	case "Stop":
		return KindDone
	case "SessionEnd":
		return KindEnd
	default:
		return ""
	}
}

// runEmit lê um payload de hook (ou JSON manual) do stdin e grava um evento.
// Nunca retorna erro fatal: hooks não devem quebrar a sessão do usuário.
func runEmit(args []string) int {
	raw, _ := io.ReadAll(os.Stdin)

	var p hookPayload
	_ = json.Unmarshal(raw, &p) // se falhar, campos ficam zerados

	kind := Kind(p.Kind)
	if kind == "" {
		kind = mapHookKind(p.HookEventName)
	}
	// Nem toda Notification é urgência: "waiting for your input" é só a sessão
	// ociosa esperando você (dispara depois do Stop) — não é "precisa de você".
	if p.HookEventName == "Notification" && isIdleWaiting(p.Message) {
		kind = KindDone
	}
	// PostToolUse dispara a CADA ferramenta. Só nos interessa pra destravar uma
	// sessão presa em "attention" (você concedeu a permissão e o Claude voltou
	// a rodar ferramentas — não há UserPromptSubmit nesse caso). Fora disso,
	// ignora: senão inundaria o log com um evento por chamada de ferramenta.
	if p.HookEventName == "PostToolUse" {
		if lastKindForSession(p.SessionID) != KindAttention {
			return 0
		}
		kind = KindWorking
	}
	// Background: Stop/SubagentStop trazem a lista de tarefas rodando. Quando a
	// sessão termina de responder mas ainda tem shell/subagente em background,
	// ela entra em "background" em vez de "done" — senão parece ociosa à toa.
	bg := runningBgTasks(p.BackgroundTasks)
	switch p.HookEventName {
	case "Stop":
		if len(bg) > 0 {
			kind = KindBackground // (mapHookKind já deu "done"; sobe pra background)
		}
	case "SubagentStop":
		// Um subagente/tarefa terminou. Só mexe se a sessão já está ociosa
		// (done/background) — se ainda está trabalhando/attention, não interfere.
		// Atualiza a contagem: volta pra "done" quando a última tarefa acaba.
		if lk := lastKindForSession(p.SessionID); lk == KindDone || lk == KindBackground {
			if len(bg) > 0 {
				kind = KindBackground
			} else {
				kind = KindDone
			}
		}
	}
	if kind == "" {
		// evento que não mapeia p/ nada útil: ignora silenciosamente
		return 0
	}

	source := p.Source
	if source == "" {
		source = "claude-code"
	}

	project := p.Project
	if project == "" && p.Cwd != "" {
		project = filepath.Base(p.Cwd)
	}
	if project == "" {
		project = "?"
	}

	title := p.Title
	if title == "" && p.TranscriptPath != "" {
		title = extractTitle(p.TranscriptPath)
	}

	var bgTasks []string
	if kind == KindBackground {
		bgTasks = bg
	}
	e := Event{
		Time:    time.Now(),
		Source:  source,
		Session: p.SessionID,
		Project: project,
		Cwd:     p.Cwd,
		Kind:    kind,
		Message: p.Message,
		Title:   title,
		BgTasks: bgTasks,
	}
	_ = appendEvent(e) // falha de gravação não deve travar o hook
	return 0
}

// extractTitle lê a transcript e retorna o último aiTitle (título humano
// que o Claude gera pra sessão). Filtra por substring antes de dar Unmarshal
// pra não parsear o arquivo inteiro linha a linha.
func extractTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	needle := []byte(`"ai-title"`)
	var last string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if !bytes.Contains(b, needle) {
			continue
		}
		var t struct {
			AiTitle string `json:"aiTitle"`
		}
		if json.Unmarshal(b, &t) == nil && t.AiTitle != "" {
			last = t.AiTitle
		}
	}
	return last
}
