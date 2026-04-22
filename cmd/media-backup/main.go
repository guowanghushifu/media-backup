package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wangdazhuo/media-backup/internal/app"
	"github.com/wangdazhuo/media-backup/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/config.example.yaml", "path to config file")
	flag.Parse()

	logFile, err := app.OpenLogFile("logs/media-backup.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal(err)
	}
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
