package initialize

import "github.com/tranducnguyen/util_core/global"

func Run() {
	InitConfig()
	InitLogger()
	InitContext()
	InitBot()
	InitProxy(global.Config.Bot.NumberBot, global.Config.Proxy.FileProxy)
}
