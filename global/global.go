package global

import (
	"context"
	"pkg/logger"
	"pkg/setting"
)

var (
	Config    setting.Config
	GContext  *context.Context
	GCanCel   *context.CancelFunc
	NumberBot int
	Logger    *logger.LoggerZap
)
