package global

import (
	"context"

	"github.com/tranducnguyen/util_core/logger"
	"github.com/tranducnguyen/util_core/setting"
)

var (
	Config    setting.Config
	GContext  *context.Context
	GCanCel   *context.CancelFunc
	NumberBot int
	Logger    *logger.LoggerZap
)
