package initialize

import (
	"pkg/proxy_pool"
)

func InitProxy(concurrency int, filepath string) {
	proxyPool := proxy_pool.NewProxyPool(concurrency, 3)
	proxyPool.LoadFromFile(filepath)
}
