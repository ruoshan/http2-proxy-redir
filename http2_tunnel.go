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

type ReadWriteHalfCloser interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	CloseRead() error
	CloseWrite() error
}

type HttpTunnel struct {
	r io.ReadCloser  // Response.Body
	w io.WriteCloser // pipe to Request.Body
}

var _ ReadWriteHalfCloser = &HttpTunnel{} // Ensure HttpTunnel implement the interface

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
	e1 := h.w.Close()
	io.ReadAll(h.r) // discard unread data
	e2 := h.r.Close()
	if e1 == nil && e2 == nil {
		return nil
	}
	return errors.New(e1.Error() + " & " + e2.Error())
}

func (h *HttpTunnel) CloseRead() error {
	io.ReadAll(h.r) // discard unread data
	return h.r.Close()
}

func (h *HttpTunnel) CloseWrite() error {
	return h.w.Close()
}

type HttpProxy struct {
	user      string
	passwd    string
	httpc     *http.Client
	transport *http2.Transport
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
		ReadIdleTimeout: 10 * time.Second,
	}
	cli := &http.Client{
		Transport: tp,
	}
	return &HttpProxy{
		user:      user,
		passwd:    passwd,
		httpc:     cli,
		transport: tp,
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

func (p *HttpProxy) CloseIdleConnections() {
	p.transport.CloseIdleConnections()
}
