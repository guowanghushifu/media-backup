package rclone

import "testing"

func TestParseStatsLine(t *testing.T) {
	t.Parallel()

	line := " \t Transferred:   	   1.234 GiB / 3.210 GiB, 38%, 12.345 MiB/s, ETA 2m12s \n"

	got, ok := ParseStats(line)
	if !ok {
		t.Fatal("ParseStats ok = false, want true")
	}

	want := "Transferred:   	   1.234 GiB / 3.210 GiB, 38%, 12.345 MiB/s, ETA 2m12s"
	if got != want {
		t.Fatalf("ParseStats() = %q, want %q", got, want)
	}
}

func TestParseStatsLineRejectsNonStatsLine(t *testing.T) {
	t.Parallel()

	got, ok := ParseStats("Checks: 12 / 12, 100%")
	if ok {
		t.Fatalf("ParseStats ok = true, want false (got %q)", got)
	}
	if got != "" {
		t.Fatalf("ParseStats() = %q, want empty string", got)
	}
}
