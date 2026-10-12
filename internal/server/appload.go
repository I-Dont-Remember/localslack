package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"localslack/internal/appmanifest"
	"localslack/internal/config"
	"localslack/internal/events"
	"localslack/internal/service"
)

// newEngine builds the real event dispatcher from the process settings.
func newEngine(cfg config.Config, svc *service.Service, log *slog.Logger) *events.Engine {
	timeout := cfg.EventTimeout
	if timeout <= 0 {
		timeout = config.DefaultEventTimeout
	}
	return events.New(events.Options{
		Source:    svc,
		Policy:    events.URLPolicy{DeliverTo: cfg.DeliverTo, AllowRemote: cfg.AllowRemoteRequestURLs},
		Config:    events.DefaultDeliveryConfig(timeout, cfg.EventRetryDelays),
		TraceSize: cfg.DeliveryLogSize,
		Log:       log,
	})
}

// registerManifests upserts every --app-manifest file: safe on each restart, and
// a stored deliver_to is never overwritten. A bad file fails startup.
func registerManifests(ctx context.Context, log *slog.Logger, cfg config.Config, svc *service.Service, disp events.Dispatcher) error {
	for _, path := range cfg.AppManifest {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("app manifest: %w", err)
		}
		parsed, err := appmanifest.Parse(data)
		if err != nil {
			return fmt.Errorf("app manifest %s: %w", path, err)
		}
		app, err := svc.RegisterApp(ctx, service.RegisterInput{Manifest: parsed, Upsert: true})
		if err != nil {
			return fmt.Errorf("app manifest %s: %w", path, err)
		}
		log.InfoContext(ctx, "app registered from manifest", slog.String("app_id", app.ID), slog.String("name", app.Name), slog.String("path", path))
		for _, w := range parsed.Warnings {
			log.WarnContext(ctx, "app manifest warning", slog.String("app_id", app.ID), slog.String("warning", w))
		}
		warnIfBlocked(ctx, log, disp, app)
	}
	return nil
}

// warnIfBlocked logs the copy-paste fix when app's events have nowhere to go.
func warnIfBlocked(ctx context.Context, log *slog.Logger, disp events.Dispatcher, app service.App) {
	ctl, ok := disp.(events.Controller)
	if !ok || app.SocketMode {
		return
	}
	if t := ctl.Resolve(app); t.Blocked {
		log.WarnContext(ctx, "app events are blocked", slog.String("app_id", app.ID), slog.String("reason", t.Reason),
			slog.String("fix", "localslack apps set-url "+app.ID+" http://localhost:3000"))
	}
}
