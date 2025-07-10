package initialize

import (
	"fmt"
	"pkg/global"

	"github.com/spf13/viper"
)

func InitConfig() {
	viper := viper.New()
	viper.AddConfigPath("./")     // path to config
	viper.SetConfigName("config") // ten file
	viper.SetConfigType("yaml")

	// read configuration
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("failed to read configuration %w", err))
	}

	if err := viper.Unmarshal(&global.Config); err != nil {
		fmt.Printf("Unable to decode configuration %v", err)
	}
}
