package main

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

type HttpTunnel struct {
	r io.ReadCloser
	w io.WriteCloser
}

func NewHttpTunnel(r io.ReadCloser, w io.WriteCloser) *HttpTunnel {
	return &HttpTunnel{r: r, w: w}
}

func (h *HttpTunnel) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

func (h *HttpTunnel) Write(p []byte) (int, error) {
	return h.w.Write(p)
}

func (h *HttpTunnel) Close() error {
	e1 := h.r.Close()
	e2 := h.w.Close()
	if e1 == nil {
		return e2
	}
	return e1
}

type HttpProxy struct {
	user   string
	passwd string
	httpc  *http.Client
}

func NewHttpProxy(proxyAddr, user, passwd string) *HttpProxy {
	tp := &http2.Transport{
		DialTLS: func(network, _ string, cfg *tls.Config) (net.Conn, error) {
			return tls.Dial(network, proxyAddr, cfg)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		PingTimeout:     3 * time.Second,
		ReadIdleTimeout: 3 * time.Second,
	}
	cli := &http.Client{
		Transport: tp,
	}
	return &HttpProxy{
		user:   user,
		passwd: passwd,
		httpc:  cli,
	}
}

func (p *HttpProxy) SetAuth(req *http.Request) error {
	req.SetBasicAuth(p.user, p.passwd)
	auth := req.Header.Get("Authorization")
	req.Header.Set("Proxy-Authorization", auth)
	req.Header.Del("Authorization")
	return nil
}

func (p *HttpProxy) DialTunnel(targetUrl string) (*HttpTunnel, error) {
	// Preamble CONNECT request
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodConnect, targetUrl, pr)
	if err != nil {
		return nil, err
	}
	p.SetAuth(req)
	resp, err := p.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusProxyAuthRequired {
		return nil, errors.New("auth failed")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("CONNECT failed")
	}
	// Preamble done
	return NewHttpTunnel(resp.Body, pw), nil
}
