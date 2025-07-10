package initialize

import "pkg/global"

func InitBot() {
	if global.Config.Bot.NumberBot == 0 {
		global.Config.Bot.NumberBot = 1
	} else {
		global.NumberBot = global.Config.Bot.NumberBot
	}

}
