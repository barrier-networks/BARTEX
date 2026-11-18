package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Engagement / Rules-of-Engagement store. An engagement is a consultancy-run
// container for one authorised piece of client work: a time window, an
// authorised allow-list and deny-list of targets, an intensity cap and a signed
// authorisation reference. The enforcement layer lives in package intercept; the
// db layer only returns plain rows/structs (GetEngagementResolved), which the
// server maps into an intercept.EngagementPolicy. Keeping the policy type out of
// db keeps the dependency direction clean (db must never import intercept).

// Engagement is one engagements row.
type Engagement struct {
	ID                 int64     `json:"id"`
	CompanyID          int64     `json:"company_id"`
	Name               string    `json:"name"`
	Status             string    `json:"status"` // draft|active|suspended|closed
	StartsAt           time.Time `json:"starts_at"`
	EndsAt             time.Time `json:"ends_at"`
	PermittedTestTypes []string  `json:"permitted_test_types"`
	IntensityCap       string    `json:"intensity_cap"` // off|quiet|balanced|aggressive
	AuthorisedByName   string    `json:"authorised_by_name"`
	AuthorisedByEmail  string    `json:"authorised_by_email"`
	AuthorisationRef   string    `json:"authorisation_ref"`
	Notes              string    `json:"notes"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// EngagementScopeRow is one engagement_scope row (an allow or a deny entry).
type EngagementScopeRow struct {
	ID           int64  `json:"id"`
	EngagementID int64  `json:"engagement_id"`
	Mode         string `json:"mode"` // allow|deny
	Kind         string `json:"kind"` // domain|subdomain|ip|cidr|cloud_account
	Domain       string `json:"domain,omitempty"`
	Net          string `json:"net,omitempty"`
	CloudAccount string `json:"cloud_account,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// EngagementInput is the data needed to create an engagement.
type EngagementInput struct {
	CompanyID          int64
	Name               string
	Status             string
	StartsAt           time.Time
	EndsAt             time.Time
	PermittedTestTypes []string
	IntensityCap       string
	AuthorisedByName   string
	AuthorisedByEmail  string
	AuthorisationRef   string
	Notes              string
	Scope              []EngagementScopeRow
}

// EngagementPatch is a partial update; nil fields are left unchanged.
type EngagementPatch struct {
	Name               *string
	Status             *string
	StartsAt           *time.Time
	EndsAt             *time.Time
	PermittedTestTypes *[]string
	IntensityCap       *string
	AuthorisedByName   *string
	AuthorisedByEmail  *string
	AuthorisationRef   *string
	Notes              *string
}

// EngagementResolved is the minimal snapshot the enforcement policy is built
// from: status, window and the allow/deny scope rows. It carries no intercept
// type, so the server maps it into an intercept.EngagementPolicy.
type EngagementResolved struct {
	ID       int64
	Status   string
	StartsAt time.Time
	EndsAt   time.Time
	Allow    []EngagementScopeRow
	Deny     []EngagementScopeRow
}

// ErrEngagementNotFound is returned when an engagement id does not resolve.
var ErrEngagementNotFound = errors.New("engagement not found")

var engagementValidStatuses = map[string]bool{"draft": true, "active": true, "suspended": true, "closed": true}
var engagementValidCaps = map[string]bool{"off": true, "quiet": true, "balanced": true, "aggressive": true}
var engagementValidScopeKinds = map[string]bool{"domain": true, "subdomain": true, "ip": true, "cidr": true, "cloud_account": true}

// marshalTestTypes serialises the permitted-test-types list to JSON for the
// JSONB column, always producing a valid array ("[]" when empty/nil).
func marshalTestTypes(types []string) string {
	if len(types) == 0 {
		return "[]"
	}
	b, err := json.Marshal(types)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// CreateEngagement inserts an engagement and its initial scope rows in one
// transaction and returns the new id.
func (d *DB) CreateEngagement(e EngagementInput) (int64, error) {
	if e.CompanyID <= 0 {
		return 0, fmt.Errorf("company_id is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return 0, fmt.Errorf("name is required")
	}
	if e.Status == "" {
		e.Status = "draft"
	}
	if !engagementValidStatuses[e.Status] {
		return 0, fmt.Errorf("invalid status %q", e.Status)
	}
	if e.IntensityCap == "" {
		e.IntensityCap = "balanced"
	}
	if !engagementValidCaps[e.IntensityCap] {
		return 0, fmt.Errorf("invalid intensity_cap %q", e.IntensityCap)
	}
	if e.StartsAt.IsZero() || e.EndsAt.IsZero() {
		return 0, fmt.Errorf("starts_at and ends_at are required")
	}
	if !e.EndsAt.After(e.StartsAt) {
		return 0, fmt.Errorf("ends_at must be after starts_at")
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`
INSERT INTO engagements(company_id, name, status, starts_at, ends_at, permitted_test_types,
    intensity_cap, authorised_by_name, authorised_by_email, authorisation_ref, notes)
VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11)
RETURNING id`, e.CompanyID, e.Name, e.Status, e.StartsAt.UTC(), e.EndsAt.UTC(),
		marshalTestTypes(e.PermittedTestTypes), e.IntensityCap, e.AuthorisedByName,
		e.AuthorisedByEmail, e.AuthorisationRef, e.Notes).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, row := range e.Scope {
		if err := insertEngagementScopeTx(tx, id, row); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// insertEngagementScopeTx validates and inserts one scope row. A duplicate
// (uq_eng_scope) is silently ignored.
func insertEngagementScopeTx(tx *sql.Tx, engagementID int64, row EngagementScopeRow) error {
	if row.Mode != "allow" && row.Mode != "deny" {
		return fmt.Errorf("scope mode must be allow or deny, got %q", row.Mode)
	}
	if !engagementValidScopeKinds[row.Kind] {
		return fmt.Errorf("invalid scope kind %q", row.Kind)
	}
	var domainVal, netVal, cloudVal, reasonVal any
	switch row.Kind {
	case "domain", "subdomain":
		if strings.TrimSpace(row.Domain) == "" {
			return fmt.Errorf("scope kind %q requires a domain", row.Kind)
		}
		domainVal = strings.ToLower(strings.TrimSpace(row.Domain))
	case "ip", "cidr":
		if strings.TrimSpace(row.Net) == "" {
			return fmt.Errorf("scope kind %q requires a net (ip/cidr)", row.Kind)
		}
		netVal = strings.TrimSpace(row.Net)
	case "cloud_account":
		if strings.TrimSpace(row.CloudAccount) == "" {
			return fmt.Errorf("scope kind cloud_account requires a cloud_account")
		}
		cloudVal = strings.TrimSpace(row.CloudAccount)
	}
	if row.Reason != "" {
		reasonVal = row.Reason
	}
	_, err := tx.Exec(`
INSERT INTO engagement_scope(engagement_id, mode, kind, domain, net, cloud_account, reason)
VALUES ($1,$2,$3,$4,$5::cidr,$6,$7)
ON CONFLICT DO NOTHING`, engagementID, row.Mode, row.Kind, domainVal, netVal, cloudVal, reasonVal)
	return err
}

// scanEngagement reads one engagement row (column order fixed by engagementCols).
func scanEngagement(sc interface{ Scan(...any) error }) (*Engagement, error) {
	var e Engagement
	var testTypes []byte
	if err := sc.Scan(&e.ID, &e.CompanyID, &e.Name, &e.Status, &e.StartsAt, &e.EndsAt,
		&testTypes, &e.IntensityCap, &e.AuthorisedByName, &e.AuthorisedByEmail,
		&e.AuthorisationRef, &e.Notes, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	if len(testTypes) > 0 {
		_ = json.Unmarshal(testTypes, &e.PermittedTestTypes)
	}
	return &e, nil
}

const engagementCols = `id, company_id, name, status, starts_at, ends_at, permitted_test_types,
intensity_cap, authorised_by_name, authorised_by_email, authorisation_ref, notes, created_at, updated_at`

// GetEngagement returns one engagement (nil + ErrEngagementNotFound if absent).
func (d *DB) GetEngagement(id int64) (*Engagement, error) {
	e, err := scanEngagement(d.QueryRow(`SELECT `+engagementCols+` FROM engagements WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEngagementNotFound
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ListEngagements returns engagements, newest first, optionally filtered by
// company (companyID<=0 = all) and status ("" = all).
func (d *DB) ListEngagements(companyID int64, status string) ([]Engagement, error) {
	q := `SELECT ` + engagementCols + ` FROM engagements WHERE 1=1`
	var args []any
	if companyID > 0 {
		args = append(args, companyID)
		q += fmt.Sprintf(" AND company_id=$%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		q += fmt.Sprintf(" AND status=$%d", len(args))
	}
	q += ` ORDER BY id DESC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Engagement{}
	for rows.Next() {
		e, err := scanEngagement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// UpdateEngagement applies a partial update. Only non-nil patch fields change.
func (d *DB) UpdateEngagement(id int64, patch EngagementPatch) error {
	var sets []string
	var args []any
	add := func(expr string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s=$%d", expr, len(args)))
	}
	if patch.Name != nil {
		add("name", *patch.Name)
	}
	if patch.Status != nil {
		if !engagementValidStatuses[*patch.Status] {
			return fmt.Errorf("invalid status %q", *patch.Status)
		}
		add("status", *patch.Status)
	}
	if patch.StartsAt != nil {
		add("starts_at", patch.StartsAt.UTC())
	}
	if patch.EndsAt != nil {
		add("ends_at", patch.EndsAt.UTC())
	}
	if patch.PermittedTestTypes != nil {
		args = append(args, marshalTestTypes(*patch.PermittedTestTypes))
		sets = append(sets, fmt.Sprintf("permitted_test_types=$%d::jsonb", len(args)))
	}
	if patch.IntensityCap != nil {
		if !engagementValidCaps[*patch.IntensityCap] {
			return fmt.Errorf("invalid intensity_cap %q", *patch.IntensityCap)
		}
		add("intensity_cap", *patch.IntensityCap)
	}
	if patch.AuthorisedByName != nil {
		add("authorised_by_name", *patch.AuthorisedByName)
	}
	if patch.AuthorisedByEmail != nil {
		add("authorised_by_email", *patch.AuthorisedByEmail)
	}
	if patch.AuthorisationRef != nil {
		add("authorisation_ref", *patch.AuthorisationRef)
	}
	if patch.Notes != nil {
		add("notes", *patch.Notes)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := d.Exec(`UPDATE engagements SET `+strings.Join(sets, ", ")+
		fmt.Sprintf(" WHERE id=$%d", len(args)), args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEngagementNotFound
	}
	return nil
}

// AddEngagementScope inserts allow/deny scope rows for an engagement and returns
// how many rows were newly inserted (duplicates are ignored).
func (d *DB) AddEngagementScope(id int64, rows []EngagementScopeRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM engagements WHERE id=$1)`, id).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrEngagementNotFound
	}
	var before, after int
	if err := tx.QueryRow(`SELECT count(*) FROM engagement_scope WHERE engagement_id=$1`, id).Scan(&before); err != nil {
		return 0, err
	}
	for _, row := range rows {
		if err := insertEngagementScopeTx(tx, id, row); err != nil {
			return 0, err
		}
	}
	if err := tx.QueryRow(`SELECT count(*) FROM engagement_scope WHERE engagement_id=$1`, id).Scan(&after); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return after - before, nil
}

// GetEngagementScope returns all scope rows for an engagement, ordered by id.
func (d *DB) GetEngagementScope(id int64) ([]EngagementScopeRow, error) {
	rows, err := d.Query(`
SELECT id, engagement_id, mode, kind, COALESCE(domain,''), COALESCE(net::text,''),
       COALESCE(cloud_account,''), COALESCE(reason,'')
FROM engagement_scope WHERE engagement_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EngagementScopeRow{}
	for rows.Next() {
		var r EngagementScopeRow
		if err := rows.Scan(&r.ID, &r.EngagementID, &r.Mode, &r.Kind, &r.Domain, &r.Net, &r.CloudAccount, &r.Reason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetTaskEngagement attaches a task to an engagement. engagementID<=0 clears the
// attachment (reverts to the no-engagement fallback). Returns whether a task row
// was updated.
func (d *DB) SetTaskEngagement(taskID, engagementID int64) (bool, error) {
	var res sql.Result
	var err error
	if engagementID <= 0 {
		res, err = d.Exec(`UPDATE tasks SET engagement_id=NULL WHERE id=$1 AND deleted_at IS NULL`, taskID)
	} else {
		var exists bool
		if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM engagements WHERE id=$1)`, engagementID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrEngagementNotFound
		}
		res, err = d.Exec(`UPDATE tasks SET engagement_id=$2 WHERE id=$1 AND deleted_at IS NULL`, taskID, engagementID)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetEngagementResolved returns the status, window and allow/deny scope rows an
// enforcement policy is built from. The server maps this into an
// intercept.EngagementPolicy (db must not import intercept).
func (d *DB) GetEngagementResolved(id int64) (*EngagementResolved, error) {
	var r EngagementResolved
	err := d.QueryRow(`SELECT id, status, starts_at, ends_at FROM engagements WHERE id=$1`, id).
		Scan(&r.ID, &r.Status, &r.StartsAt, &r.EndsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEngagementNotFound
	}
	if err != nil {
		return nil, err
	}
	scope, err := d.GetEngagementScope(id)
	if err != nil {
		return nil, err
	}
	for _, row := range scope {
		switch row.Mode {
		case "allow":
			r.Allow = append(r.Allow, row)
		case "deny":
			r.Deny = append(r.Deny, row)
		}
	}
	return &r, nil
}
