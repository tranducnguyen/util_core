# util_core

A lightweight Go utility core providing common initialization scaffolding for apps and services: configuration (Viper), structured logging (Zap with lumberjack rotation), application context lifecycle, bot settings, proxy pool management, HTTP helpers, file I/O helpers, and more.

This library is intended to be imported in your Go applications to bootstrap consistent, configurable runtime behavior with minimal code.

## Features
- Configuration loader using Viper (YAML) mapped to strongly-typed structs
- Structured logging via Zap with rotation (lumberjack) and configurable levels
- Global application context with cancel function
- Bot settings bootstrap (number of workers)
- Proxy pool with file-based loading and concurrency control
- Helpers for HTTP, data, proxy, and file operations

## Requirements
- Go 1.23+

## Installation
```
go get github.com/tranducnguyen/util_core
```

## Quick Start
1) Create a config.yaml in your application working directory:

```yaml
logger:
  log_level: info
  file_log_name: ./logs/log.txt
  max_backups: 5
  max_age: 7
  max_size: 1024
  compress: true

bot:
  bot_name: my-bot
  number_bot: 4
  worklist: ./data/wordlist.txt

proxy:
  proxy_type: http
  file_proxy: ./data/proxies.txt
```

2) Initialize in your main application:

```go
package main

import (
    "github.com/tranducnguyen/util_core/initialize"
    "github.com/tranducnguyen/util_core/global"
)

func main() {
    // Run all bootstraps: config, logger, context, bot, proxy
    initialize.Run()

    // Use global logger
    global.Logger.Info("application started")

    // Use global context
    // ctx := *global.GContext

    // ... your app logic ...
}
```

3) Run your app. The library expects config.yaml in the current working directory. Paths in config.yaml can be relative to the working directory.

## Package Overview
- initialize
  - InitConfig: loads YAML config into global.Config
  - InitLogger: builds a Zap logger using settings or defaults
  - InitContext: creates a cancellable root context stored in global
  - InitBot: sets global.NumberBot from config with safe defaults
  - InitProxy: builds and loads a proxy pool from file
  - Run: orchestrates all the above in order
- global
  - Exposes shared singletons: Config, Logger, GContext, GCanCel, NumberBot
- setting
  - Defines Config, LoggerSetting, BotSetting, ProxySetting and mapstructure tags
- proxy_pool, helper, filehandle, client, bot
  - Supporting utilities for proxies, HTTP, I/O, and bot flows

## Configuration Reference
Mapping follows setting.Config with mapstructure tags.

logger:
- log_level: string (debug|info|warn|error), default: info
- file_log_name: path to log file, default: ./logs/log.txt
- max_backups: int, default: 5
- max_age: days, default: 7
- max_size: MB, default: 1024
- compress: bool, default: true

bot:
- bot_name: string
- number_bot: int, default: 1
- worklist: path to file (note: mapped from "worklist" in struct tag)

proxy:
- proxy_type: string (e.g., http, socks5)
- file_proxy: path to a proxies list file

Notes:
- initialize.InitConfig uses Viper to read ./config.yaml. If not present, it will panic at startup. Ensure your working directory contains config.yaml or adjust your process starting directory.
- initialize.InitLogger applies defaults if fields are empty/zero in the config. Logger is stored in global.Logger.
- initialize.InitProxy creates a proxy pool with concurrency equal to bot.number_bot and loads proxies from proxy.file_proxy.

## Error Handling
- If configuration loading fails, InitConfig will panic with context. Ensure YAML is present and valid.
- Logger initialization falls back to defaults for any missing fields.

## Development
- Module path: github.com/tranducnguyen/util_core
- Go version: 1.23

## Contributing
Issues and PRs are welcome. Please format code with gofmt and keep changes minimal and modular.

## License
Specify your license here. If none is specified, this code is provided as-is.

## Support
If you encounter issues, please open an issue on the repository and include your Go version, OS, a minimal config.yaml, and reproduction steps.
