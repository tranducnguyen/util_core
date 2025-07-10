package initialize

import "pkg/global"

func Run() {
	InitConfig()
	InitLogger()
	InitContext()
	InitBot()
	InitProxy(global.Config.Bot.NumberBot, global.Config.Proxy.FileProxy)
}
