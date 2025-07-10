package initialize

import (
	"context"
	"pkg/global"
)

func InitContext() {
	ctx, cancel := context.WithCancel(context.Background())
	global.GContext = &ctx
	global.GCanCel = &cancel
}
