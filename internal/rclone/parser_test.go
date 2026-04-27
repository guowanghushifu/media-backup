package rclone

import "testing"

func TestParseStatsLine(t *testing.T) {
	t.Parallel()

	line := "2026/04/22 17:04:10 INFO  :       832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"

	got, ok := ParseStats(line)
	if !ok {
		t.Fatal("ParseStats ok = false, want true")
	}

	want := "832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s"
	if got != want {
		t.Fatalf("ParseStats() = %q, want %q", got, want)
	}
}

func TestParseStatsLineRejectsLegacyTransferredFormat(t *testing.T) {
	t.Parallel()

	line := " \t Transferred:   	   1.234 GiB / 3.210 GiB, 38%, 12.345 MiB/s, ETA 2m12s \n"

	got, ok := ParseStats(line)
	if ok {
		t.Fatalf("ParseStats ok = true, want false (got %q)", got)
	}
}

func TestParseStatsLineRejectsNonStatsLines(t *testing.T) {
	t.Parallel()

	lines := []string{
		"Checks: 12 / 12, 100%",
		"2026/04/22 17:04:17 INFO  : THIS_IS_TEST/uploadtest.bin: Copied (new)",
		"2026/04/22 17:04:18 INFO  : THIS_IS_TEST: Set directory modification time (using DirSetModTime)",
	}

	for _, line := range lines {
		got, ok := ParseStats(line)
		if ok {
			t.Fatalf("ParseStats ok = true, want false (got %q)", got)
		}
		if got != "" {
			t.Fatalf("ParseStats() = %q, want empty string", got)
		}
	}
}

func TestParseInfoPayload(t *testing.T) {
	t.Parallel()

	line := "2026/04/22 17:04:17 INFO  : THIS_IS_TEST/uploadtest.bin: Copied (new)"
	got, ok := ParseInfoPayload(line)
	if !ok {
		t.Fatal("ParseInfoPayload ok = false, want true")
	}
	want := "THIS_IS_TEST/uploadtest.bin: Copied (new)"
	if got != want {
		t.Fatalf("ParseInfoPayload() = %q, want %q", got, want)
	}
}
