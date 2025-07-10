package proxy_pool

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"pkg/global"
	"pkg/helper"
	"pkg/types"
	"sync"
	"time"
)

// ProxyStatus represents the status of a proxy
type ProxyStatus int

const (
	StatusUnknown ProxyStatus = iota
	StatusWorking
	StatusFailed
	StatusBlacklisted
)

var (
	ProxoPoolInstance *ProxyPool
	once              sync.Once
	TIME_RELOAD       = 1 * time.Minute
)

func GetInstance() *ProxyPool {
	once.Do(func() {
		proxyPool := NewProxyPool(global.NumberBot, 3)
		proxyPool.LoadFromFile("proxy.txt")
		proxyPool.monitor()
		ProxoPoolInstance = proxyPool
	})
	return ProxoPoolInstance
}

// ProxyInfo represents a proxy with its status and metadata
type ProxyInfo struct {
	Proxy       *types.Proxy
	Address     string
	Status      ProxyStatus
	LastUsed    time.Time
	FailCount   int
	SuccessRate float64
	InUse       bool
}

// ProxyPool manages a pool of proxies with concurrency control
type ProxyPool struct {
	proxies     []*ProxyInfo
	whitelist   chan *ProxyInfo
	blacklist   map[string]bool
	mutex       sync.RWMutex
	semaphore   chan struct{}
	currentIdx  int
	maxFailures int
}

// NewProxyPool creates a new proxy pool with specified concurrency
func NewProxyPool(concurrency int, maxFailures int) *ProxyPool {
	return &ProxyPool{
		proxies:     make([]*ProxyInfo, 0),
		blacklist:   make(map[string]bool),
		whitelist:   make(chan *ProxyInfo, concurrency),
		semaphore:   make(chan struct{}, concurrency),
		currentIdx:  0,
		maxFailures: maxFailures,
		mutex:       sync.RWMutex{},
	}
}

// LoadFromFile loads proxies from a file with one proxy per line
func (p *ProxyPool) LoadFromFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return p.LoadFromReader(file)
}

// LoadFromReader loads proxies from an io.Reader
func (p *ProxyPool) LoadFromReader(r io.Reader) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Skip blacklisted proxies
		if p.blacklist[line] {
			continue
		}

		p.proxies = append(p.proxies, &ProxyInfo{
			Address:   line,
			Status:    StatusUnknown,
			LastUsed:  time.Time{},
			FailCount: 0,
			InUse:     false,
		})
	}

	return scanner.Err()
}

// AddProxy adds a single proxy to the pool
func (p *ProxyPool) AddProxy(proxyAddr string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Skip if already blacklisted
	if p.blacklist[proxyAddr] {
		return
	}

	// Check if proxy already exists
	for _, proxy := range p.proxies {
		if proxy.Address == proxyAddr {
			return
		}
	}

	p.proxies = append(p.proxies, &ProxyInfo{
		Address:   proxyAddr,
		Status:    StatusUnknown,
		LastUsed:  time.Time{},
		FailCount: 0,
		InUse:     false,
	})
}

// BlacklistProxy adds a proxy to the blacklist
func (p *ProxyPool) BlacklistProxy(proxyAddr string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.blacklist[proxyAddr] = true

	// Remove proxy from active pool if it exists
	for i, proxy := range p.proxies {
		if proxy.Address == proxyAddr {
			proxy.Status = StatusBlacklisted
			// Remove from active proxies
			p.proxies = append(p.proxies[:i], p.proxies[i+1:]...)
			break
		}
	}
}

// IsBlacklisted checks if a proxy is blacklisted
func (p *ProxyPool) IsBlacklisted(proxyAddr string) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.blacklist[proxyAddr]
}

// GetNextProxy gets the next available proxy from the pool
func (p *ProxyPool) GetNextProxy() (*ProxyInfo, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if len(p.proxies) == 0 {
		return nil, errors.New("proxy pool is empty")
	}
	proxy, err := p.GetWhitelist()
	if err == nil {
		return proxy, nil
	}

	// Try to find a non-in-use proxy
	startIdx := p.currentIdx
	for {
		p.currentIdx = (p.currentIdx + 1) % len(p.proxies)
		proxy := p.proxies[p.currentIdx]

		// Skip blacklisted or in-use proxies
		if p.blacklist[proxy.Address] || proxy.InUse {
			// If we've checked all proxies and come back to where we started
			if p.currentIdx == startIdx {
				return nil, errors.New("all proxies are currently in use")
			}
			continue
		}

		// Mark proxy as in-use
		proxy.InUse = true
		proxy.LastUsed = time.Now()
		return proxy, nil
	}
}

func (p *ProxyPool) GetWhitelist() (*ProxyInfo, error) {
	if len(p.whitelist) == 0 {
		return nil, errors.New("whitelist is empty")
	}

	proxy := <-p.whitelist
	proxy.InUse = true
	proxy.LastUsed = time.Now()
	return proxy, nil
}

func (p *ProxyPool) AddToWhitelist(proxy *ProxyInfo) {
	p.whitelist <- proxy
}

// GetProxy acquires a proxy and the semaphore for concurrency control
// It blocks until a semaphore slot is available
func (p *ProxyPool) GetProxy() (*ProxyInfo, error) {
	// Acquire a semaphore slot (blocks if all slots are in use)
	p.semaphore <- struct{}{}
	proxy, err := p.GetNextProxy()
	if err != nil {
		<-p.semaphore
		return nil, err
	}
	return proxy, nil
}

// ReleaseProxy releases a proxy back to the pool and updates its status
func (p *ProxyPool) ReleaseProxy(proxy *ProxyInfo, success *bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if proxy == nil {
		return
	}

	fmt.Printf("Release %s, %v", proxy.Address, *success)
	if *success {
		p.AddToWhitelist(proxy)
	}

	// Find the proxy in our list
	for i, existingProxy := range p.proxies {
		if existingProxy.Address == proxy.Address {
			if *success {
				existingProxy.Status = StatusWorking
				existingProxy.FailCount = 0
			} else {
				existingProxy.FailCount++
				if existingProxy.FailCount >= p.maxFailures {
					existingProxy.Status = StatusFailed
					// Optionally blacklist after too many failures
					p.blacklist[existingProxy.Address] = true
					// Remove from active proxies
					p.proxies = append(p.proxies[:i], p.proxies[i+1:]...)
				}
			}
			existingProxy.InUse = false
			break
		}
	}

	<-p.semaphore
}

// Size returns the number of active proxies in the pool
func (p *ProxyPool) Size() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.proxies)
}

// BlacklistSize returns the number of blacklisted proxies
func (p *ProxyPool) BlacklistSize() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.blacklist)
}

// ClearBlacklist clears the proxy blacklist
func (p *ProxyPool) ClearBlacklist() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.blacklist = make(map[string]bool)
}

// GetProxyStats returns statistics about the proxy pool
func (p *ProxyPool) GetProxyStats() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["total_proxies"] = len(p.proxies)
	stats["blacklisted_proxies"] = len(p.blacklist)

	statusCounts := map[ProxyStatus]int{
		StatusUnknown:     0,
		StatusWorking:     0,
		StatusFailed:      0,
		StatusBlacklisted: 0,
	}

	inUseCount := 0
	for _, proxy := range p.proxies {
		statusCounts[proxy.Status]++
		if proxy.InUse {
			inUseCount++
		}
	}

	stats["status_counts"] = statusCounts
	stats["in_use_count"] = inUseCount

	return stats
}

func (p *ProxyPool) monitor() {
	go func() {
		for {
			now := time.Now()
			for _, proxy := range p.proxies {
				if now.Sub(proxy.LastUsed) > TIME_RELOAD {
					p.blacklist[proxy.Address] = false
				}
			}
			time.Sleep(TIME_RELOAD)
		}
	}()
}

func (pi *ProxyInfo) IsParse() bool {
	if pi.Proxy == nil {
		return false
	}

	if pi.Proxy.Host == "" || pi.Proxy.Port == "" || pi.Proxy.Scheme == "" {
		return false
	}

	return true
}

func (pi *ProxyInfo) DoParseUrl() error {
	if !pi.IsParse() {
		proxyObj, err := helper.ParseProxy(pi.Address)
		if err != nil {
			return fmt.Errorf("failed to parse proxy URL: %w", err)
		}
		pi.Proxy = proxyObj
	}
	return nil
}
