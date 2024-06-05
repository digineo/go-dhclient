package dhclient

import (
	"context"
	"log/slog"
)

type discardHandler struct {
	slog.JSONHandler
}

func (d *discardHandler) Enabled(context.Context, slog.Level) bool {
	return false
}
