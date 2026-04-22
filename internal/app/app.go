package app

import "context"

type Dependencies struct {
	RunUploads func(context.Context) error
}

type App struct {
	runUploads func(context.Context) error
}

func New(deps Dependencies) *App {
	return &App{runUploads: deps.RunUploads}
}

func (a *App) Run(parent context.Context, stop <-chan struct{}) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	if a.runUploads == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return a.runUploads(ctx)
}
