package app

import "context"

// Called on the UI thread before starting a configuration job. Canceling here
// also invalidates a queued older job that has not reached FileIndex.Configure.
func (a *App) restartFileIndexContext() context.Context {
	if a.fileIndexCancel != nil {
		a.fileIndexCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.fileIndexCancel = cancel
	return ctx
}
