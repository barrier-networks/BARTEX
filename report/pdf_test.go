package report

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// TestRenderPDFSmoke renders a trivial document through the headless-Chromium
// engine and checks the output is a PDF. It skips where no browser is installed
// so it is safe in CI/air-gapped environments.
func TestRenderPDFSmoke(t *testing.T) {
	pdf, err := RenderPDF(context.Background(), []byte("<!doctype html><html><body><h1>BARTEX</h1></body></html>"))
	if errors.Is(err, ErrNoBrowser) {
		t.Skip("no Chromium available for PDF rendering")
	}
	if err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		n := 16
		if len(pdf) < n {
			n = len(pdf)
		}
		t.Fatalf("output is not a PDF (prefix %q)", pdf[:n])
	}
	if len(pdf) < 500 {
		t.Errorf("suspiciously small PDF: %d bytes", len(pdf))
	}
}
