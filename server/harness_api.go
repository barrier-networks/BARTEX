package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/barrier-networks/bartex/db"
)

// harnessProfileInput is the editable shape the UI sends. Env is a plain map on
// the way in (sealed at rest by the store); on the way out only the key names
// are returned (HarnessProfile.EnvKeys), never the values.
type harnessProfileInput struct {
	Name                 string            `json:"name"`
	Kind                 string            `json:"kind"`
	Command              string            `json:"command"`
	Args                 []string          `json:"args"`
	Env                  map[string]string `json:"env"`
	MCPTransport         string            `json:"mcp_transport"`
	DisableNativeSandbox *bool             `json:"disable_native_sandbox"`
	Daybreak             bool              `json:"daybreak"`
	MaxTurns             int               `json:"max_turns"`
	MaxDurationSeconds   int               `json:"max_duration_seconds"`
}

func (s *Server) listHarnessProfiles(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	ps, err := s.m.pg.ListHarnessProfiles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if ps == nil {
		ps = []*db.HarnessProfile{}
	}
	writeJSON(w, 200, ps)
}

func (s *Server) createHarnessProfile(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	var in harnessProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	p := profileFromInput(in, nil)
	id, err := s.m.pg.SaveHarnessProfile(p)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) updateHarnessProfile(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in harnessProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	p := profileFromInput(in, &id)
	if _, err := s.m.pg.SaveHarnessProfile(p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id})
}

func (s *Server) deleteHarnessProfile(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.m.pg.DeleteHarnessProfile(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// attachTaskHarness selects (or clears) a task's external-harness backend.
// POST /api/tasks/{id}/harness  {"harness_profile_id": <id|null>}
func (s *Server) attachTaskHarness(w http.ResponseWriter, r *http.Request) {
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok || t == nil {
		writeErr(w, 404, "task not found")
		return
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		writeErr(w, 400, "bad task id")
		return
	}
	var body struct {
		HarnessProfileID *int64 `json:"harness_profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if err := s.m.pg.SetTaskHarnessProfile(id, body.HarnessProfileID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// Reflect the change on the in-memory task so the next intent dispatch sees it.
	if body.HarnessProfileID != nil {
		v := *body.HarnessProfileID
		t.HarnessProfileID = &v
	} else {
		t.HarnessProfileID = nil
	}
	writeJSON(w, 200, map[string]any{"ok": true, "harness_profile_id": body.HarnessProfileID})
}

// profileFromInput maps the request onto a db.HarnessProfile. A nil id is an
// insert; on update, a nil Env in the request (the UI omitting it) leaves the
// stored env untouched, while an explicit (even empty) map replaces it.
func profileFromInput(in harnessProfileInput, id *int64) *db.HarnessProfile {
	p := &db.HarnessProfile{
		Name:               strings.TrimSpace(in.Name),
		Kind:               strings.TrimSpace(in.Kind),
		Command:            strings.TrimSpace(in.Command),
		Args:               in.Args,
		Env:                in.Env,
		MCPTransport:       strings.TrimSpace(in.MCPTransport),
		Daybreak:           in.Daybreak,
		MaxTurns:           in.MaxTurns,
		MaxDurationSeconds: in.MaxDurationSeconds,
	}
	// Default the fail-safe ON (confine the CLI; its own sandbox off) unless the
	// caller explicitly set it.
	p.DisableNativeSandbox = in.DisableNativeSandbox == nil || *in.DisableNativeSandbox
	if id != nil {
		p.ID = *id
	}
	return p
}
