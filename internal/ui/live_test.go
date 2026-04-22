package ui

import "testing"

func TestEnterAlternateScreen(t *testing.T) {
	t.Parallel()

	if got := EnterAlternateScreen(); got != "\x1b[?1049h" {
		t.Fatalf("EnterAlternateScreen() = %q, want %q", got, "\x1b[?1049h")
	}
}

func TestLeaveAlternateScreen(t *testing.T) {
	t.Parallel()

	if got := LeaveAlternateScreen(); got != "\x1b[?1049l" {
		t.Fatalf("LeaveAlternateScreen() = %q, want %q", got, "\x1b[?1049l")
	}
}

func TestRewriteFrameRepaintsFromHome(t *testing.T) {
	t.Parallel()

	content := "line1\nline2"
	want := "\x1b[H\x1b[J" + content
	if got := RewriteFrame(content); got != want {
		t.Fatalf("RewriteFrame() = %q, want %q", got, want)
	}
}
