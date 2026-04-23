package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/guowanghushifu/media-backup/internal/app"
	"github.com/guowanghushifu/media-backup/internal/config"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	logDir, err := resolveLogDir(os.Executable)
	if err != nil {
		log.Fatal(err)
	}

	logWriter := app.NewDailyLogWriter(logDir, time.Now)
	defer logWriter.Close()

	logger := log.New(logWriter, "", log.LstdFlags)

	resolvedConfigPath, err := resolveConfigPath(*configPath, os.Executable, os.Stat)
	if err != nil {
		logger.Fatal(err)
	}

	cfg, err := config.LoadConfig(resolvedConfigPath)
	if err != nil {
		logger.Fatal(err)
	}
	logConfigPath(logger, resolvedConfigPath)
	service, err := app.NewService(cfg, logger)
	if err != nil {
		logger.Fatal(err)
	}

	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		close(stop)
	}()

	a := app.New(app.Dependencies{
		RunUploads: service.Run,
	})
	if err := a.Run(context.Background(), stop); err != nil && err != context.Canceled {
		logger.Fatal(err)
	}
}

func resolveConfigPath(flagValue string, executable func() (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	exePath, err := executable()
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(filepath.Dir(exePath), "config.yaml")
	if _, err := stat(candidate); err == nil {
		return candidate, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return "", errors.New("config file not found: specify -config or place config.yaml next to the executable")
}

func resolveLogDir(executable func() (string, error)) (string, error) {
	exePath, err := executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exePath), "logs"), nil
}

func logConfigPath(logger *log.Logger, path string) {
	if logger == nil {
		return
	}
	logger.Printf("using config file: %s", path)
}
