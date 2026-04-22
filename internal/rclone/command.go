package rclone

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/wangdazhuo/media-backup/internal/config"
)

type CommandExecutor struct {
	Binary   string
	Proxy    config.ProxyConfig
	OnOutput func(string)
}

func (e *CommandExecutor) Run(ctx context.Context, args []string) error {
	binary := e.Binary
	if binary == "" {
		binary = "rclone"
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	env, err := commandEnv(e.Proxy)
	if err != nil {
		return err
	}
	cmd.Env = env

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

func commandEnv(proxy config.ProxyConfig) ([]string, error) {
	env := append([]string(nil), os.Environ()...)
	proxyVars, err := proxyEnv(proxy)
	if err != nil {
		return nil, err
	}
	return append(env, proxyVars...), nil
}

func proxyEnv(proxy config.ProxyConfig) ([]string, error) {
	if !proxy.Enabled {
		return nil, nil
	}

	u := &url.URL{
		Scheme: proxy.Scheme,
		Host:   net.JoinHostPort(proxy.Host, strconv.Itoa(proxy.Port)),
	}
	switch {
	case proxy.Username != "" && proxy.Password != "":
		u.User = url.UserPassword(proxy.Username, proxy.Password)
	case proxy.Username != "":
		u.User = url.User(proxy.Username)
	}

	proxyURL := u.String()
	return []string{
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
	}, nil
}
