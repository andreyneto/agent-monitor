package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// isIdleWaiting reconhece a notificação "Claude is waiting for your input",
// que significa sessão ociosa esperando você — não é "precisa de você".
func isIdleWaiting(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "waiting for your input")
}

// Kind é o status normalizado de uma sessão.
type Kind string

const (
	KindStart     Kind = "start"     // sessão iniciou
	KindWorking   Kind = "working"   // recebeu um prompt / está trabalhando
	KindAttention Kind = "attention" // PRECISA de você (permissão / input)
	KindDone      Kind = "done"      // terminou de responder, esperando você
	KindEnd       Kind = "end"       // sessão encerrada
)

// Event é uma linha do events.jsonl. Qualquer ferramenta pode empurrar um
// evento; o Claude Code faz isso via hooks (ver emit.go).
type Event struct {
	Time    time.Time `json:"time"`
	Source  string    `json:"source"`  // "claude-code", "gemini", "cursor"...
	Session string    `json:"session"` // id único da sessão
	Project string    `json:"project"` // nome curto (basename do cwd)
	Cwd     string    `json:"cwd"`
	Kind    Kind      `json:"kind"`
	Message string    `json:"message"`
	Title   string    `json:"title,omitempty"` // aiTitle da sessão, quando já existe
}

// monDir é onde vivem os eventos. Override com MON_DIR.
func monDir() string {
	if d := os.Getenv("MON_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "monitor")
}

func eventsPath() string { return filepath.Join(monDir(), "events.jsonl") }

// appendEvent grava uma linha JSON de forma atômica (O_APPEND + um Write só).
func appendEvent(e Event) error {
	if err := os.MkdirAll(monDir(), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(eventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line) // um Write único é atômico p/ tamanhos pequenos no POSIX
	return err
}

// readEvents lê todo o log. Linhas corrompidas são ignoradas.
func readEvents() ([]Event, error) {
	f, err := os.Open(eventsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var e Event
		if json.Unmarshal(b, &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// lastKindForSession devolve o Kind do último evento de uma sessão, lendo só o
// fim do arquivo (barato mesmo com o log grande). Devolve "" se não achar.
// Usado pelo hook PostToolUse pra decidir se precisa destravar um "attention".
func lastKindForSession(session string) Kind {
	if session == "" {
		return ""
	}
	f, err := os.Open(eventsPath())
	if err != nil {
		return ""
	}
	defer f.Close()

	// lê os últimos ~64KB (cobre centenas de eventos recentes)
	const window = 64 * 1024
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := int64(0)
	if fi.Size() > window {
		off = fi.Size() - window
	}
	if _, err := f.Seek(off, 0); err != nil {
		return ""
	}

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if off > 0 {
		sc.Scan() // descarta a 1ª linha (provavelmente cortada no meio)
	}
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	// varre de trás pra frente: o 1º match é o evento mais recente da sessão
	for i := len(lines) - 1; i >= 0; i-- {
		var e Event
		if json.Unmarshal([]byte(lines[i]), &e) != nil || e.Session != session {
			continue
		}
		return curedKind(e)
	}

	// não achou no tail: a sessão pode estar presa em "attention" há tempo,
	// enquanto outras sessões empurraram o evento pra fora da janela de 64KB.
	// Esse é justamente o caso que precisamos destravar — vale o full-read
	// (raro: só quando o último evento da sessão está fora do tail).
	if off > 0 {
		if events, err := readEvents(); err == nil {
			for i := len(events) - 1; i >= 0; i-- {
				if events[i].Session == session {
					return curedKind(events[i])
				}
			}
		}
	}
	return ""
}

// curedKind aplica a mesma cura de deriveSessions: um "attention" que na verdade
// é "waiting for your input" conta como done, não como urgência.
func curedKind(e Event) Kind {
	if e.Kind == KindAttention && isIdleWaiting(e.Message) {
		return KindDone
	}
	return e.Kind
}

// Session é o estado derivado (último evento) de uma sessão.
type Session struct {
	ID       string
	Source   string
	Project  string
	Cwd      string
	Kind     Kind
	Message  string
	Title    string
	LastSeen time.Time
}

// deriveSessions reduz o log de eventos ao estado atual por sessão.
// Sessões encerradas (KindEnd) são removidas.
func deriveSessions(events []Event) map[string]*Session {
	sessions := map[string]*Session{}
	for _, e := range events {
		if e.Session == "" {
			continue
		}
		if e.Kind == KindEnd {
			delete(sessions, e.Session)
			continue
		}
		// cura eventos antigos: attention "waiting for input" não é urgência
		if e.Kind == KindAttention && isIdleWaiting(e.Message) {
			e.Kind = KindDone
		}
		s := sessions[e.Session]
		if s == nil {
			s = &Session{ID: e.Session}
			sessions[e.Session] = s
		}
		s.Source = e.Source
		s.Project = e.Project
		s.Cwd = e.Cwd
		s.Kind = e.Kind
		s.Message = e.Message
		s.LastSeen = e.Time
		if e.Title != "" { // título é "sticky": nem todo evento carrega
			s.Title = e.Title
		}
	}
	return sessions
}
