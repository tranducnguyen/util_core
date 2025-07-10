package initialize

import (
	"pkg/global"
	"pkg/logger"
	"pkg/setting"
)

func InitLogger() {
	settingLogger := setting.LoggerSetting{
		Log_level:     "info",
		File_log_name: "./logs/log.txt",
		Max_backups:   5,
		Max_age:       7,
		Max_size:      1024,
		Compress:      true,
	}
	if global.Config.Logger.Log_level != "" {
		settingLogger.Log_level = global.Config.Logger.Log_level
	}
	if global.Config.Logger.File_log_name != "" {
		settingLogger.File_log_name = global.Config.Logger.File_log_name
	}
	if global.Config.Logger.Max_backups != 0 {
		settingLogger.Max_backups = global.Config.Logger.Max_backups
	}
	if global.Config.Logger.Max_age != 0 {
		settingLogger.Max_age = global.Config.Logger.Max_age
	}
	if global.Config.Logger.Max_size != 0 {
		settingLogger.Max_size = global.Config.Logger.Max_size
	}
	if global.Config.Logger.Compress {
		settingLogger.Compress = global.Config.Logger.Compress
	}
	// Check if the logger settings are empty
	// If empty, use the default logger settings

	global.Logger = logger.NewLogger(settingLogger)
}
