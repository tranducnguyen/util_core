package helper

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/tranducnguyen/util_core/global"
	"github.com/tranducnguyen/util_core/types"
)

// ParseProxy parses a proxy string in various formats and returns a Proxy struct
// Supported formats:
// - scheme://host:port
// - scheme://username:password@host:port
// - host:port (assumes http scheme)
// - username:password@host:port (assumes http scheme)
func ParseProxy(proxyStr string) (*types.Proxy, error) {
	if proxyStr == "" {
		return nil, errors.New("empty proxy string")
	}

	proxy := &types.Proxy{
		Available: true,
		Scheme:    global.Config.Proxy.ProxyType, // Default scheme
	}

	// Check if the proxy string already has a scheme
	hasScheme := strings.Contains(proxyStr, "://")

	// If no scheme is specified, prepend "http://" for parsing
	if !hasScheme {
		proxyStr = fmt.Sprintf("%s://%s", global.Config.Proxy.ProxyType, proxyStr)
	}

	// Parse the URL
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
	}

	// Extract scheme (protocol)
	if hasScheme {
		proxy.Scheme = proxyURL.Scheme
	}

	// Extract host and port
	hostPort := proxyURL.Host
	if hostPort == "" {
		return nil, errors.New("missing host in proxy string")
	}

	// Split host and port
	host, port, err := splitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	proxy.Host = host
	proxy.Port = port

	// Extract username and password if present
	if proxyURL.User != nil {
		proxy.Username = proxyURL.User.Username()
		password, passwordSet := proxyURL.User.Password()
		if passwordSet {
			proxy.Password = password
		}
	}

	return proxy, nil
}

// splitHostPort separates host and port from a string in format "host:port"
func splitHostPort(hostPort string) (string, string, error) {
	parts := strings.Split(hostPort, ":")

	if len(parts) != 2 {
		return "", "", errors.New("invalid host:port format")
	}

	host := parts[0]
	port := parts[1]

	if host == "" {
		return "", "", errors.New("empty host")
	}

	if port == "" {
		return "", "", errors.New("empty port")
	}

	return host, port, nil
}
