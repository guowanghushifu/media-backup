package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/guowanghushifu/media-backup/internal/config"
)

const telegramNotifyTimeout = 5 * time.Second

type jobFailureNotification struct {
	JobName    string
	LinkPath   string
	RetryCount int
	LastError  string
}

type telegramNotifier struct {
	client   *http.Client
	botToken string
	chatID   string
}

func newTelegramNotifier(cfg config.TelegramConfig, proxy config.ProxyConfig) *telegramNotifier {
	if !cfg.Enabled {
		return nil
	}
	client := &http.Client{Timeout: telegramNotifyTimeout}
	if proxyURL := telegramProxyURL(proxy); proxyURL != nil {
		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}
	return &telegramNotifier{
		client:   client,
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
	}
}

func telegramProxyURL(proxy config.ProxyConfig) *url.URL {
	if !proxy.Enabled {
		return nil
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
	return u
}

func (n *telegramNotifier) NotifyFinalFailure(ctx context.Context, event jobFailureNotification) error {
	if n == nil {
		return nil
	}

	body := map[string]string{
		"chat_id": n.chatID,
		"text": fmt.Sprintf(
			"media-backup final failure\njob: %s\nfile: %s\nretries: %d\nerror: %s",
			event.JobName,
			event.LinkPath,
			event.RetryCount,
			event.LastError,
		),
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+n.botToken+"/sendMessage", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed with status %s", resp.Status)
	}
	return nil
}
