package setting

type Config struct {
	Logger LoggerSetting `mapstructure:"logger"`
	Bot    BotSetting    `mapstructure:"bot"`
	Proxy  ProxySetting  `mapstructure:"proxy"`
}

type LoggerSetting struct {
	Log_level     string `mapstructure:"log_level"`
	File_log_name string `mapstructure:"file_log_name"`
	Max_backups   int    `mapstructure:"max_backups"`
	Max_age       int    `mapstructure:"max_age"`
	Max_size      int    `mapstructure:"max_size"`
	Compress      bool   `mapstructure:"compress"`
}

type BotSetting struct {
	BotName   string `mapstructure:"bot_name"`
	NumberBot int    `mapstructure:"number_bot"`
	Wordlist  string `mapstructure:"worklist"`
}

type ProxySetting struct {
	ProxyType string `mapstructure:"proxy_type"`
	FileProxy string `mapstructure:"file_proxy"`
}
