package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wangdazhuo/media-backup/internal/app"
)

func main() {
	logFile, err := app.OpenLogFile("logs/media-backup.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		close(stop)
	}()

	a := app.New(app.Dependencies{
		RunUploads: func(context.Context) error { return nil },
	})
	if err := a.Run(context.Background(), stop); err != nil && err != context.Canceled {
		logger.Fatal(err)
	}
}
