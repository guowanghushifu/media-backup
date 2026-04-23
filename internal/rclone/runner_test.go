package rclone

import (
	"context"
	"errors"
	"testing"
)

func TestRunnerBuildsCommand(t *testing.T) {
	t.Parallel()

	exec := &RecordingExecutor{}
	runner := NewRunner(exec)

	extraArgs := []string{"--stats=1s", "--stats-one-line", "-v"}
	if err := runner.Copy(context.Background(), "/links/movies", "remote:movies", extraArgs); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	want := []string{"copy", "/links/movies", "remote:movies", "--stats=1s", "--stats-one-line", "-v"}
	if len(exec.Args) != len(want) {
		t.Fatalf("command arg length = %d, want %d", len(exec.Args), len(want))
	}
	for i := range want {
		if exec.Args[i] != want[i] {
			t.Fatalf("command arg[%d] = %q, want %q", i, exec.Args[i], want[i])
		}
	}
}

func TestRunnerForwardsExecutorError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("copy failed")
	exec := &RecordingExecutor{Err: wantErr}
	runner := NewRunner(exec)

	err := runner.Copy(context.Background(), "/links/movies", "remote:movies", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Copy() error = %v, want %v", err, wantErr)
	}
}

func TestRunnerBuildsSingleFileCopyCommand(t *testing.T) {
	t.Parallel()

	exec := &RecordingExecutor{}
	runner := NewRunner(exec)

	extraArgs := []string{"--stats=1s", "--stats-one-line", "-v"}
	if err := runner.CopyFile(context.Background(), "/links/movies/feature.mkv", "gd1:/sync/movies/", extraArgs); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	want := []string{"copy", "/links/movies/feature.mkv", "gd1:/sync/movies/", "--stats=1s", "--stats-one-line", "-v"}
	if len(exec.Args) != len(want) {
		t.Fatalf("command arg length = %d, want %d", len(exec.Args), len(want))
	}
	for i := range want {
		if exec.Args[i] != want[i] {
			t.Fatalf("command arg[%d] = %q, want %q", i, exec.Args[i], want[i])
		}
	}
}

type RecordingExecutor struct {
	Args []string
	Err  error
}

func (r *RecordingExecutor) Run(_ context.Context, args []string) error {
	r.Args = append([]string(nil), args...)
	return r.Err
}
