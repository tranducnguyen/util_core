package clientpool

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/tranducnguyen/util_core/types"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

// ClientPool manages fasthttp clients by proxy address
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*fasthttp.Client
	// Track last usage time for cleanup
	lastUsed map[string]time.Time
	// Max idle time before a client is removed from the pool (10 minutes)
	maxIdleTime time.Duration
}

// Global client pool instance
var (
	globalClientPool     *ClientPool
	globalClientPoolOnce sync.Once
)

// GetClientPool returns the global client pool instance
func GetClientPool() *ClientPool {
	globalClientPoolOnce.Do(func() {
		globalClientPool = &ClientPool{
			clients:     make(map[string]*fasthttp.Client),
			lastUsed:    make(map[string]time.Time),
			maxIdleTime: 2 * time.Minute, // 10 minutes max idle time
		}
		// Start cleanup goroutine
		go globalClientPool.startCleanup()
	})
	return globalClientPool
}

// startCleanup periodically removes idle clients from the pool
func (p *ClientPool) startCleanup() {
	ticker := time.NewTicker(2 * time.Minute) // Check every 2 minutes
	defer ticker.Stop()

	for range ticker.C {
		p.cleanupIdleClients()
	}
}

// cleanupIdleClients removes clients that haven't been used for a while
func (p *ClientPool) cleanupIdleClients() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for key, lastUsed := range p.lastUsed {
		if now.Sub(lastUsed) > p.maxIdleTime {
			if client, ok := p.clients[key]; ok {
				// Close any idle connections before removing
				client.CloseIdleConnections()
				delete(p.clients, key)
				delete(p.lastUsed, key)
			}
		}
	}
}

// GetClient returns a client for the given proxy, creating a new one if needed
func (p *ClientPool) GetClient(proxy *types.Proxy) *fasthttp.Client {
	// Generate unique key for this proxy configuration
	var key string
	if proxy == nil {
		key = "direct"
	} else {
		key = proxy.GetProxyURL()
	}

	// Try to get existing client
	p.mu.RLock()
	client, exists := p.clients[key]
	p.mu.RUnlock()

	if exists {
		// Update last used time
		p.mu.Lock()
		p.lastUsed[key] = time.Now()
		p.mu.Unlock()
		return client
	}

	// Create new client if needed
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check if another goroutine created the client while we were waiting for the lock
	if client, exists = p.clients[key]; exists {
		p.lastUsed[key] = time.Now()
		return client
	}

	// Create new client
	var dialFunc fasthttp.DialFunc
	if proxy != nil {
		dialFunc = fasthttpproxy.FasthttpHTTPDialerTimeout(proxy.GetProxyURL(), 30*time.Second)
	} else {
		dialFunc = fasthttp.Dial
	}

	client = &fasthttp.Client{
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		Dial: dialFunc,
		// Add other client configurations as needed
		MaxIdleConnDuration: 1 * time.Minute,
		MaxConnDuration:     60 * time.Second,
		ReadTimeout:         30 * time.Second,
		WriteTimeout:        30 * time.Second,
	}

	p.clients[key] = client
	p.lastUsed[key] = time.Now()
	return client
}

// ReleaseClient updates the last used time for a client
// This should be called after a client is done being used
func (p *ClientPool) ReleaseClient(proxy *types.Proxy) {
	var key string
	if proxy == nil {
		key = "direct"
	} else {
		key = proxy.GetProxyURL()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.clients[key]; exists {
		p.lastUsed[key] = time.Now()
	}
}

// Close closes all clients and removes them from the pool
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, client := range p.clients {
		client.CloseIdleConnections()
		delete(p.clients, key)
		delete(p.lastUsed, key)
	}
}

// AcquireClient gets a client from the pool instead of creating a new one each time
// This is a drop-in replacement for the existing AcquireClient function
func AcquireClient(proxy *types.Proxy) *fasthttp.Client {
	return GetClientPool().GetClient(proxy)
}

// When you're done using a client, call this to update its last used time
// Although fasthttp clients can be used concurrently, this helps the pool
// track which clients are actively being used
func ReleaseClient(proxy *types.Proxy) {
	GetClientPool().ReleaseClient(proxy)
}
