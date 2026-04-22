package ui

import "testing"

func TestRewriteFrameFirstRender(t *testing.T) {
	t.Parallel()

	got, lines := RewriteFrame(0, "line1\nline2")
	if got != "line1\nline2" {
		t.Fatalf("RewriteFrame() output = %q", got)
	}
	if lines != 2 {
		t.Fatalf("RewriteFrame() lines = %d, want 2", lines)
	}
}

func TestRewriteFrameRefreshesExistingRegion(t *testing.T) {
	t.Parallel()

	got, lines := RewriteFrame(3, "line1\nline2")
	want := "\r\x1b[2A\x1b[Jline1\nline2"
	if got != want {
		t.Fatalf("RewriteFrame() output = %q, want %q", got, want)
	}
	if lines != 2 {
		t.Fatalf("RewriteFrame() lines = %d, want 2", lines)
	}
}
