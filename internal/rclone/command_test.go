package rclone

import (
	"reflect"
	"testing"

	"github.com/wangdazhuo/media-backup/internal/config"
)

func TestProxyEnvBuildsURLWithEscapedCredentials(t *testing.T) {
	t.Parallel()

	proxy := config.ProxyConfig{
		Enabled:  true,
		Scheme:   "http",
		Host:     "127.0.0.1",
		Port:     7890,
		Username: "demo@user",
		Password: "p@ss:word",
	}

	got, err := proxyEnv(proxy)
	if err != nil {
		t.Fatalf("proxyEnv() error = %v", err)
	}

	wantURL := "http://demo%40user:p%40ss%3Aword@127.0.0.1:7890"
	want := []string{
		"HTTP_PROXY=" + wantURL,
		"HTTPS_PROXY=" + wantURL,
		"http_proxy=" + wantURL,
		"https_proxy=" + wantURL,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("proxyEnv() = %#v, want %#v", got, want)
	}
}

func TestProxyEnvReturnsNilWhenDisabled(t *testing.T) {
	t.Parallel()

	got, err := proxyEnv(config.ProxyConfig{})
	if err != nil {
		t.Fatalf("proxyEnv() error = %v", err)
	}
	if got != nil {
		t.Fatalf("proxyEnv() = %#v, want nil", got)
	}
}
