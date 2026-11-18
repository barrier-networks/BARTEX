package report

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrNoBrowser is returned by RenderPDF when no headless-capable Chromium binary
// can be found. Callers should surface this as "PDF rendering unavailable" (and
// can still serve the HTML report, which needs no browser).
var ErrNoBrowser = errors.New("no Chromium binary found for PDF rendering")

// chromiumCandidates are searched in order. BARRIER_CHROMIUM wins; the container
// image ships a preinstalled Chromium under /opt/pw-browsers.
func chromiumCandidates() []string {
	var c []string
	if env := os.Getenv("BARRIER_CHROMIUM"); env != "" {
		c = append(c, env)
	}
	c = append(c, "/opt/pw-browsers/chromium")
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			c = append(c, p)
		}
	}
	return c
}

func findChromium() (string, bool) {
	for _, p := range chromiumCandidates() {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}

// RenderPDF converts an HTML document to PDF bytes by driving a headless
// Chromium (print-to-pdf). It needs no cgo and no Go dependency; the browser is a
// discovered external runtime. Returns ErrNoBrowser when none is available.
func RenderPDF(ctx context.Context, html []byte) ([]byte, error) {
	bin, ok := findChromium()
	if !ok {
		return nil, ErrNoBrowser
	}
	dir, err := os.MkdirTemp("", "barrier-report-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "report.html")
	out := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(in, html, 0o600); err != nil {
		return nil, err
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}

	// A private user-data dir keeps runs isolated; --no-sandbox is required to run
	// headless Chromium as root inside a container.
	cmd := exec.CommandContext(ctx, bin,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--no-pdf-header-footer",
		"--user-data-dir="+filepath.Join(dir, "profile"),
		"--print-to-pdf="+out,
		"file://"+in,
	)
	if combined, err := cmd.CombinedOutput(); err != nil {
		// Retry once with the legacy --headless flag for older Chromium builds.
		cmd = exec.CommandContext(ctx, bin, "--headless", "--no-sandbox", "--disable-gpu",
			"--user-data-dir="+filepath.Join(dir, "profile2"), "--print-to-pdf="+out, "file://"+in)
		if combined2, err2 := cmd.CombinedOutput(); err2 != nil {
			return nil, fmt.Errorf("chromium print-to-pdf failed: %v: %s / %s", err2, combined, combined2)
		}
	}
	pdf, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("reading rendered pdf: %w", err)
	}
	if len(pdf) == 0 {
		return nil, errors.New("chromium produced an empty pdf")
	}
	return pdf, nil
}
