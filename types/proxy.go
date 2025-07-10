package types

import "fmt"

type Proxy struct {
	Available      bool   `json:"available"`
	AverageLatency int64  `json:"average_latency"`
	Errors         int64  `json:"errors"`
	Host           string `json:"host"`
	Password       string `json:"password"`
	Port           string `json:"port"`
	Requests       int64  `json:"requests"`
	Scheme         string `json:"scheme"`
	Username       string `json:"username"`
}

func (p *Proxy) GetProxyURL() string {
	var auth string
	if p.Username != "" {
		if p.Password != "" {
			auth = fmt.Sprintf("%s:%s@", p.Username, p.Password)
		} else {
			auth = fmt.Sprintf("%s@", p.Username)
		}
	}

	return fmt.Sprintf("%s://%s%s:%s", p.Scheme, auth, p.Host, p.Port)
}
