package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/barrier-networks/bartex/db"
	"github.com/barrier-networks/bartex/report"
)

// taskReportMeta builds the client-report document context from query
// parameters, so the same report can be regenerated with different framing
// (client name, classification, authoriser) without changing the findings.
func taskReportMeta(r *http.Request) report.ClientReportMeta {
	q := r.URL.Query()
	return report.ClientReportMeta{
		Client:         q.Get("client"),
		Title:          q.Get("title"),
		Classification: q.Get("classification"),
		Author:         q.Get("author"),
		Reference:      q.Get("ref"),
		EngagementName: q.Get("engagement"),
		ScopeSummary:   q.Get("scope"),
		PeriodStart:    q.Get("period_start"),
		PeriodEnd:      q.Get("period_end"),
	}
}

// taskReportFindings loads a single task's findings, most-severe first. A blank
// or invalid id is rejected by the caller so an empty filter can never return
// every task's findings.
func (s *Server) taskReportFindings(id string) ([]*db.DBFinding, error) {
	return s.m.pg.ListFindingsForExport(db.FindingFilter{TaskID: id, Sort: "severity"}, nil)
}

func validTaskID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		writeErr(w, 400, "invalid task id")
		return "", false
	}
	return id, true
}

// getTaskReportHTML renders the Barrier-branded client report for one task as
// HTML (no browser dependency; always available).
func (s *Server) getTaskReportHTML(w http.ResponseWriter, r *http.Request) {
	id, ok := validTaskID(w, r)
	if !ok {
		return
	}
	fs, err := s.taskReportFindings(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	html, err := report.ClientReportHTML(fs, taskReportMeta(r), time.Now())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// getTaskReportPDF renders the same report to PDF via headless Chromium. When no
// browser is available it returns 501 and points at the HTML report, which needs
// none.
func (s *Server) getTaskReportPDF(w http.ResponseWriter, r *http.Request) {
	id, ok := validTaskID(w, r)
	if !ok {
		return
	}
	fs, err := s.taskReportFindings(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	html, err := report.ClientReportHTML(fs, taskReportMeta(r), time.Now())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	pdf, err := report.RenderPDF(r.Context(), []byte(html))
	if err != nil {
		if errors.Is(err, report.ErrNoBrowser) {
			writeErr(w, 501, "PDF rendering is unavailable: no Chromium binary was found. Use the HTML report (report.html), or set BARRIER_CHROMIUM to a Chromium executable.")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="barrier-report-task-`+id+`.pdf"`)
	_, _ = w.Write(pdf)
}
