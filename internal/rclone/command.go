package rclone

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
)

type CommandExecutor struct {
	Binary   string
	OnOutput func(string)
}

func (e *CommandExecutor) Run(ctx context.Context, args []string) error {
	binary := e.Binary
	if binary == "" {
		binary = "rclone"
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go e.scanPipe(&wg, stdout)
	go e.scanPipe(&wg, stderr)
	wg.Wait()
	return cmd.Wait()
}

func (e *CommandExecutor) scanPipe(wg *sync.WaitGroup, r io.Reader) {
	defer wg.Done()

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		if e.OnOutput != nil {
			e.OnOutput(scanner.Text())
		}
	}
}
