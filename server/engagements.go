package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/barrier-networks/bartex/db"
	"github.com/barrier-networks/bartex/intercept"
)

// Engagement / Rules-of-Engagement HTTP API + the server-side bridge that builds
// the enforcement policy (package server maps db rows into intercept types so the
// db package never imports intercept). Phase 1: create / list / detail / scope /
// status update, plus attach/detach a task to an engagement. UI is Phase 2.

// buildEngagementPolicy resolves an engagement and builds the read-only
// enforcement snapshot the guard evaluates per tool call. A resolve error is
// propagated so the caller (Task.engagementPolicy) fails safe (blocking) without
// caching the error.
func (m *Manager) buildEngagementPolicy(id int64) (*intercept.EngagementPolicy, error) {
	r, err := m.pg.GetEngagementResolved(id)
	if err != nil {
		return nil, err
	}
	return engagementPolicyFromResolved(r), nil
}

// engagementPolicyFromResolved maps a db snapshot into an intercept.EngagementPolicy.
func engagementPolicyFromResolved(r *db.EngagementResolved) *intercept.EngagementPolicy {
	p := &intercept.EngagementPolicy{
		EngagementID: r.ID,
		Status:       r.Status,
		StartsAt:     r.StartsAt,
		EndsAt:       r.EndsAt,
	}
	for _, row := range r.Allow {
		if n, ok := engagementNetFromRow(row); ok {
			p.Allow = append(p.Allow, n)
		}
	}
	for _, row := range r.Deny {
		n, ok := engagementNetFromRow(row)
		if !ok {
			// A deny row we cannot parse is an exclusion we cannot enforce.
			// Dropping it would silently widen reach, so fail closed: mark the
			// policy unresolved and every call is blocked (ambiguous) until the
			// exclusion is fixed. (A dropped allow row only shrinks reach, so it
			// is safe to skip above.)
			p.Unresolved = true
			continue
		}
		p.Deny = append(p.Deny, n)
	}
	return p
}

// engagementNetFromRow converts one scope row into an intercept.EngagementNet.
// A row that cannot be parsed (an invalid net, an empty domain) is dropped; a
// dropped allow row simply never authorises, and a dropped deny row cannot
// exclude — both only shrink or keep reach, never widen it.
func engagementNetFromRow(row db.EngagementScopeRow) (intercept.EngagementNet, bool) {
	n := intercept.EngagementNet{Kind: row.Kind}
	switch row.Kind {
	case "domain", "subdomain":
		n.Domain = strings.ToLower(strings.TrimSpace(row.Domain))
		return n, n.Domain != ""
	case "ip", "cidr":
		raw := strings.TrimSpace(row.Net)
		if raw == "" {
			return n, false
		}
		if pfx, err := netip.ParsePrefix(raw); err == nil {
			n.Net = pfx.Masked()
			return n, true
		}
		if a, err := netip.ParseAddr(raw); err == nil {
			n.Net = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
			return n, true
		}
		return n, false
	case "cloud_account":
		n.CloudAccount = strings.TrimSpace(row.CloudAccount)
		return n, n.CloudAccount != ""
	}
	return n, false
}

// AttachTaskEngagement attaches a task to an engagement (engagementID<=0 /
// detachID detaches), persists it, and refreshes the live task's cached policy so
// the next tool call rebuilds it. Returns the live task (nil when not found).
func (m *Manager) AttachTaskEngagement(taskID string, engagementID int64) (*Task, error) {
	m.taskStateMu.Lock()
	defer m.taskStateMu.Unlock()
	id, err := strconv.ParseInt(taskID, 10, 64)
	if err != nil || id <= 0 {
		return nil, nil
	}
	updated, err := m.pg.SetTaskEngagement(id, engagementID)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, nil
	}
	m.mu.RLock()
	task := m.tasks[taskID]
	m.mu.RUnlock()
	if task == nil {
		return nil, nil
	}
	if engagementID <= 0 {
		task.setEngagement(nil)
	} else {
		task.setEngagement(&engagementID)
	}
	return task, nil
}

// ---- DTOs ----

type engagementScopeRowReq struct {
	Mode         string `json:"mode"`
	Kind         string `json:"kind"`
	Domain       string `json:"domain,omitempty"`
	Net          string `json:"net,omitempty"`
	CloudAccount string `json:"cloud_account,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func (r engagementScopeRowReq) toRow() db.EngagementScopeRow {
	return db.EngagementScopeRow{
		Mode: strings.TrimSpace(r.Mode), Kind: strings.TrimSpace(r.Kind),
		Domain: r.Domain, Net: r.Net, CloudAccount: r.CloudAccount, Reason: r.Reason,
	}
}

type createEngagementReq struct {
	CompanyID          int64                   `json:"company_id"`
	Name               string                  `json:"name"`
	Status             string                  `json:"status,omitempty"`
	StartsAt           string                  `json:"starts_at"`
	EndsAt             string                  `json:"ends_at"`
	PermittedTestTypes []string                `json:"permitted_test_types,omitempty"`
	IntensityCap       string                  `json:"intensity_cap,omitempty"`
	AuthorisedByName   string                  `json:"authorised_by_name,omitempty"`
	AuthorisedByEmail  string                  `json:"authorised_by_email,omitempty"`
	AuthorisationRef   string                  `json:"authorisation_ref,omitempty"`
	Notes              string                  `json:"notes,omitempty"`
	Scope              []engagementScopeRowReq `json:"scope,omitempty"`
}

func parseEngagementTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("time is required")
	}
	return time.Parse(time.RFC3339, s)
}

func (s *Server) createEngagement(w http.ResponseWriter, r *http.Request) {
	var req createEngagementReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	starts, err := parseEngagementTime(req.StartsAt)
	if err != nil {
		writeErr(w, 400, "invalid starts_at (RFC3339): "+err.Error())
		return
	}
	ends, err := parseEngagementTime(req.EndsAt)
	if err != nil {
		writeErr(w, 400, "invalid ends_at (RFC3339): "+err.Error())
		return
	}
	scope := make([]db.EngagementScopeRow, 0, len(req.Scope))
	for _, s := range req.Scope {
		scope = append(scope, s.toRow())
	}
	id, err := s.m.PG().CreateEngagement(db.EngagementInput{
		CompanyID: req.CompanyID, Name: strings.TrimSpace(req.Name), Status: req.Status,
		StartsAt: starts, EndsAt: ends, PermittedTestTypes: req.PermittedTestTypes,
		IntensityCap: req.IntensityCap, AuthorisedByName: req.AuthorisedByName,
		AuthorisedByEmail: req.AuthorisedByEmail, AuthorisationRef: req.AuthorisationRef,
		Notes: req.Notes, Scope: scope,
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) listEngagements(w http.ResponseWriter, r *http.Request) {
	var companyID int64
	if v := strings.TrimSpace(r.URL.Query().Get("company_id")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, 400, "invalid company_id")
			return
		}
		companyID = n
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	list, err := s.m.PG().ListEngagements(companyID, status)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"engagements": list})
}

func (s *Server) getEngagement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	e, err := s.m.PG().GetEngagement(id)
	if errors.Is(err, db.ErrEngagementNotFound) {
		writeErr(w, 404, "engagement not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	scope, err := s.m.PG().GetEngagementScope(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"engagement": e, "scope": scope})
}

type updateEngagementReq struct {
	Name               *string   `json:"name,omitempty"`
	Status             *string   `json:"status,omitempty"`
	StartsAt           *string   `json:"starts_at,omitempty"`
	EndsAt             *string   `json:"ends_at,omitempty"`
	PermittedTestTypes *[]string `json:"permitted_test_types,omitempty"`
	IntensityCap       *string   `json:"intensity_cap,omitempty"`
	AuthorisedByName   *string   `json:"authorised_by_name,omitempty"`
	AuthorisedByEmail  *string   `json:"authorised_by_email,omitempty"`
	AuthorisationRef   *string   `json:"authorisation_ref,omitempty"`
	Notes              *string   `json:"notes,omitempty"`
}

func (s *Server) updateEngagement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	var req updateEngagementReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	patch := db.EngagementPatch{
		Name: req.Name, Status: req.Status, IntensityCap: req.IntensityCap,
		PermittedTestTypes: req.PermittedTestTypes, AuthorisedByName: req.AuthorisedByName,
		AuthorisedByEmail: req.AuthorisedByEmail, AuthorisationRef: req.AuthorisationRef, Notes: req.Notes,
	}
	if req.StartsAt != nil {
		t, err := parseEngagementTime(*req.StartsAt)
		if err != nil {
			writeErr(w, 400, "invalid starts_at (RFC3339): "+err.Error())
			return
		}
		patch.StartsAt = &t
	}
	if req.EndsAt != nil {
		t, err := parseEngagementTime(*req.EndsAt)
		if err != nil {
			writeErr(w, 400, "invalid ends_at (RFC3339): "+err.Error())
			return
		}
		patch.EndsAt = &t
	}
	if err := s.m.PG().UpdateEngagement(id, patch); err != nil {
		if errors.Is(err, db.ErrEngagementNotFound) {
			writeErr(w, 404, "engagement not found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	// Live tasks cache their policy; it is rebuilt when they are reloaded. A status
	// change via this endpoint takes effect for a running task on its next restart
	// (Phase-1 behaviour; mid-run invalidation is a Phase-2 item).
	e, err := s.m.PG().GetEngagement(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"engagement": e})
}

func (s *Server) addEngagementScope(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	var req struct {
		Scope []engagementScopeRowReq `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	if len(req.Scope) == 0 {
		writeErr(w, 400, "scope must contain at least one row")
		return
	}
	rows := make([]db.EngagementScopeRow, 0, len(req.Scope))
	for _, s := range req.Scope {
		rows = append(rows, s.toRow())
	}
	added, err := s.m.PG().AddEngagementScope(id, rows)
	if err != nil {
		if errors.Is(err, db.ErrEngagementNotFound) {
			writeErr(w, 404, "engagement not found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"added": added})
}

// attachTaskEngagement attaches or detaches a task to/from an engagement. A null
// or 0 engagement_id detaches (reverting to the no-engagement fallback).
func (s *Server) attachTaskEngagement(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if _, ok := s.m.Task(taskID); !ok {
		writeErr(w, 404, "task not found")
		return
	}
	var req struct {
		EngagementID *int64 `json:"engagement_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	var engID int64
	if req.EngagementID != nil {
		engID = *req.EngagementID
	}
	task, err := s.m.AttachTaskEngagement(taskID, engID)
	if err != nil {
		if errors.Is(err, db.ErrEngagementNotFound) {
			writeErr(w, 400, "engagement not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if task == nil {
		writeErr(w, 404, "task not found")
		return
	}
	writeJSON(w, 200, taskDTO(task, s.resolvedTaskStatus(task)))
}
