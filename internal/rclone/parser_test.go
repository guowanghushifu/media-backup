package rclone

import "testing"

func TestParseStatsLine(t *testing.T) {
	t.Parallel()

	line := "Transferred:   	   1.234 GiB / 3.210 GiB, 38%, 12.345 MiB/s, ETA 2m12s"

	got, ok := ParseStats(line)
	if !ok {
		t.Fatal("ParseStats ok = false, want true")
	}

	want := "1.234 GiB / 3.210 GiB, 38%, 12.345 MiB/s, ETA 2m12s"
	if got != want {
		t.Fatalf("ParseStats() = %q, want %q", got, want)
	}
}
