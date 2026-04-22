package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wangdazhuo/media-backup/internal/app"
)

func TestRunCancelsUploaderOnShutdown(t *testing.T) {
	t.Parallel()

	called := false
	stop := make(chan struct{})
	a := app.New(app.Dependencies{
		RunUploads: func(ctx context.Context) error {
			<-ctx.Done()
			called = true
			return ctx.Err()
		},
	})

	go close(stop)
	err := a.Run(context.Background(), stop)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if !called {
		t.Fatal("RunUploads should observe cancellation")
	}
}
