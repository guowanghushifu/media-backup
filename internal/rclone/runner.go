package rclone

import "context"

type Executor interface {
	Run(ctx context.Context, args []string) error
}

type Runner struct {
	executor Executor
}

func NewRunner(executor Executor) *Runner {
	return &Runner{executor: executor}
}

func (r *Runner) Copy(ctx context.Context, linkDir, remote string, extraArgs []string) error {
	args := []string{"copy", linkDir, remote}
	args = append(args, extraArgs...)
	return r.executor.Run(ctx, args)
}
