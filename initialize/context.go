package initialize

import (
	"context"

	"github.com/tranducnguyen/util_core/global"
)

func InitContext() {
	ctx, cancel := context.WithCancel(context.Background())
	global.GContext = &ctx
	global.GCanCel = &cancel
}
