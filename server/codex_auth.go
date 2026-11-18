package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Codex CLI authentication, driven from System → Harness. The Codex binary and a
// persistent CODEX_HOME live in the container image; these endpoints run
// `codex login …` as the server user so the harness backend can use the login.
// Two flows: device-code (ChatGPT sign-in, headless — the UI shows a URL + code)
// and an API key piped to the CLI.

var (
	ansiRe       = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	codexCodeRe  = regexp.MustCompile(`\b([A-Z0-9]{4}-[A-Z0-9]{5})\b`)
	codexURLRe   = regexp.MustCompile(`https://\S*auth\.openai\.com/codex/device\S*`)
	codexDeviceC = 20 * time.Second // how long to wait for the URL + code to appear
)

// codexDeviceSession tracks one in-progress device-code login subprocess.
type codexDeviceSession struct {
	URL     string
	Code    string
	Started time.Time
	Done    bool   // the login completed (success or failure)
	OK      bool   // completed successfully
	Err     string // failure detail
	cancel  context.CancelFunc
}

type codexAuth struct {
	mu     sync.Mutex
	device *codexDeviceSession
}

func newCodexAuth() *codexAuth { return &codexAuth{} }

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// codexLoggedIn runs `codex login status` and reports whether a login exists.
func (s *Server) codexLoggedIn(ctx context.Context) (bool, string) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(cctx, "codex", "login", "status").CombinedOutput()
	text := stripANSI(strings.TrimSpace(string(out)))
	// `codex login status` prints "Not logged in" when unauthenticated (exit 0 in
	// both states on the probed version), so key off the text.
	loggedIn := text != "" && !strings.Contains(strings.ToLower(text), "not logged in")
	return loggedIn, lastMeaningfulLine(text)
}

func lastMeaningfulLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" && !strings.HasPrefix(l, "WARNING:") {
			return l
		}
	}
	return s
}

// codexAuthStatus: GET /api/harness/codex/status — the Codex login state plus any
// in-progress device session (so the UI can show "waiting for authorization").
func (s *Server) codexAuthStatus(w http.ResponseWriter, r *http.Request) {
	loggedIn, detail := s.codexLoggedIn(r.Context())
	resp := map[string]any{"logged_in": loggedIn, "detail": detail}
	s.codex.mu.Lock()
	if d := s.codex.device; d != nil && !d.Done {
		resp["pending"] = map[string]any{"url": d.URL, "code": d.Code, "started_at": d.Started.Unix()}
	} else if d != nil && d.Done && !d.OK && d.Err != "" {
		resp["last_error"] = d.Err
	}
	s.codex.mu.Unlock()
	writeJSON(w, 200, resp)
}

// codexDeviceLogin: POST /api/harness/codex/device-login — start (or restart) a
// device-code login and return the URL + one-time code for the operator to
// complete in a browser. The subprocess keeps running (polling) until the
// operator authorizes or the code expires.
func (s *Server) codexDeviceLogin(w http.ResponseWriter, r *http.Request) {
	s.codex.mu.Lock()
	if prev := s.codex.device; prev != nil && prev.cancel != nil && !prev.Done {
		prev.cancel() // supersede an earlier attempt
	}
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute) // code expires in 15
	sess := &codexDeviceSession{Started: time.Now(), cancel: cancel}
	s.codex.device = sess
	s.codex.mu.Unlock()

	cmd := exec.CommandContext(ctx, "codex", "login", "--device-auth")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		writeErr(w, 500, "could not start codex: "+err.Error())
		return
	}
	cmd.Stderr = cmd.Stdout // codex prints the code to stdout; merge stderr in case
	if err := cmd.Start(); err != nil {
		cancel()
		writeErr(w, 500, "could not start codex login: "+err.Error())
		return
	}

	// Read output until we have both the URL and the code (or we give up). The
	// goroutine keeps draining after we respond and records the final result.
	found := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := stripANSI(sc.Text())
			s.codex.mu.Lock()
			if sess.URL == "" {
				if m := codexURLRe.FindString(line); m != "" {
					sess.URL = m
				}
			}
			if sess.Code == "" {
				if m := codexCodeRe.FindStringSubmatch(line); m != nil {
					sess.Code = m[1]
				}
			}
			ready := sess.URL != "" && sess.Code != ""
			s.codex.mu.Unlock()
			if ready {
				select {
				case <-found:
				default:
					close(found)
				}
			}
		}
		werr := cmd.Wait()
		loggedIn, _ := s.codexLoggedIn(context.Background())
		s.codex.mu.Lock()
		sess.Done = true
		sess.OK = werr == nil && loggedIn
		if !sess.OK && werr != nil {
			sess.Err = werr.Error()
		}
		s.codex.mu.Unlock()
		cancel()
	}()

	select {
	case <-found:
	case <-time.After(codexDeviceC):
	}

	s.codex.mu.Lock()
	url, code := sess.URL, sess.Code
	s.codex.mu.Unlock()
	if url == "" || code == "" {
		writeErr(w, 502, "codex did not return a device code (device-code sign-in may need enabling in your ChatGPT security settings)")
		return
	}
	writeJSON(w, 200, map[string]any{"url": url, "code": code, "expires_minutes": 15})
}

// codexAPIKeyLogin: POST /api/harness/codex/api-key {api_key} — authenticate Codex
// with an API key (piped to the CLI; never logged).
func (s *Server) codexAPIKeyLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	key := strings.TrimSpace(body.APIKey)
	if key == "" {
		writeErr(w, 400, "api_key is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "login", "--with-api-key")
	cmd.Stdin = strings.NewReader(key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeErr(w, 502, "codex login failed: "+lastMeaningfulLine(stripANSI(string(out))))
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// codexLogout: POST /api/harness/codex/logout — clear the stored Codex login.
func (s *Server) codexLogout(w http.ResponseWriter, r *http.Request) {
	s.codex.mu.Lock()
	if d := s.codex.device; d != nil && d.cancel != nil && !d.Done {
		d.cancel()
	}
	s.codex.device = nil
	s.codex.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "codex", "logout").Run()
	writeJSON(w, 200, map[string]any{"ok": true})
}
