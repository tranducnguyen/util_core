package initialize

import (
	"github.com/tranducnguyen/util_core/proxy_pool"
)

func InitProxy(concurrency int, filepath string) {
	proxyPool := proxy_pool.NewProxyPool(concurrency, 3)
	proxyPool.LoadFromFile(filepath)
}
