package main

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

type ProxyProvider interface {
	Name() string
	Ready() bool
	DialTunnel(targetUrl string) (*HttpTunnel, error)
}

type ProxyGroup struct {
	proxies []ProxyProvider
	mu      sync.Mutex
}

func NewProxyGroup(addrs string) *ProxyGroup {
	lst := strings.Split(addrs, ",")
	providers := make([]ProxyProvider, 0, len(lst))
	for _, a := range lst {
		proxy := NewHttpProxy(a, user, passwd)
		proxy.Config(
			WithTimeout(timeout),
			WithBackoffThreshold(backoff),
		)
		providers = append(providers, proxy)
	}
	return &ProxyGroup{
		proxies: providers,
	}
}

func (pg *ProxyGroup) DialTunnel(targetUrl string) (*HttpTunnel, error) {
	// only reorder when the first is not healthy and the second is good
	if len(pg.proxies) > 1 && !pg.proxies[0].Ready() && pg.proxies[1].Ready() {
		pg.reorderByHealth()
	}

	for _, p := range pg.proxies {
		if p.Ready() {
			return p.DialTunnel(targetUrl)
		}
	}
	return nil, errors.New("Failed to find a healthy proxy")
}

// put all ready proxy in the front
func (pg *ProxyGroup) reorderByHealth() {
	pg.mu.Lock()
	sort.SliceStable(pg.proxies, func(i, j int) bool {
		if pg.proxies[i].Ready() == pg.proxies[j].Ready() {
			return false
		}
		return pg.proxies[i].Ready()
	})
	pg.mu.Unlock()
}

func (pg *ProxyGroup) Head() string {
	return pg.proxies[0].Name()
}
