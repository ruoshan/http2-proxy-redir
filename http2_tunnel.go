package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"net"
	"net/http"
	"reflect"
	"time"
	"unsafe"

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

// A hacky way to get the underlying ClientConnPool in net/http2 pkg
// and change the connection cache key to proxy's addr for every req
type clientConnPoolMock struct {
	origPool http2.ClientConnPool
}

var _ http2.ClientConnPool = &clientConnPoolMock{}

// MockClientConnPool use unsafe reflection to change the http2.Transport's
// connPool by wrap the original connPool with the GetClientConn method changed.
// the method is changed to always use fixed mocked addrs as cache key
func MockClientConnPool(tp *http2.Transport, proxyAddr string) {
	tp.CloseIdleConnections() // Calling this method only to initialize the transport's default client pool
	v := reflect.ValueOf(tp).Elem()
	f := v.FieldByName("connPoolOrDef")
	orig := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	cp := &clientConnPoolMock{
		origPool: orig.Interface().(http2.ClientConnPool),
	}
	orig.Set(reflect.ValueOf(cp))
}

const upstreamConnNum = 5

// Ignore the _addr, use one of the mockAddrs to create multiple TCP conns (increase throughput)
func (cp *clientConnPoolMock) GetClientConn(req *http.Request, _addr string) (*http2.ClientConn, error) {
	mrand.Seed(time.Now().UnixNano())
	i := mrand.Intn(upstreamConnNum)
	mockAddr := fmt.Sprintf("mock-%d:80", i)
	return cp.origPool.GetClientConn(req, mockAddr)
}

func (cp *clientConnPoolMock) MarkDead(c *http2.ClientConn) {
	cp.origPool.MarkDead(c)
}

type HttpProxy struct {
	user      string
	passwd    string
	proxyAddr string
	httpc     *http.Client
	transport *http2.Transport
	backoff   bool
	bkoff_ch  chan struct{}
	bkoff_n   int // threshold
	timeout   time.Duration
}

func NewHttpProxy(proxyAddr, user, passwd string) *HttpProxy {
	tp := &http2.Transport{
		DialTLS: func(network, _addr string, cfg *tls.Config) (net.Conn, error) {
			// Ignore the _addr, use proxyAddr instead
			return tls.Dial(network, proxyAddr, cfg)
		},
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		PingTimeout:     3 * time.Second,
		ReadIdleTimeout: 10 * time.Second,
	}
	MockClientConnPool(tp, proxyAddr)
	cli := &http.Client{
		Transport: tp,
	}
	p := &HttpProxy{
		user:      user,
		passwd:    passwd,
		proxyAddr: proxyAddr,
		httpc:     cli,
		transport: tp,
		backoff:   false,
		bkoff_ch:  make(chan struct{}, 5),
		bkoff_n:   3,
		timeout:   10 * time.Second,
	}
	go p.backoff_watchdog()
	return p
}

// Configs helper
type cfgFunc func(*HttpProxy)

func WithTimeout(sec int) cfgFunc {
	return func(p *HttpProxy) {
		p.timeout = time.Duration(sec) * time.Second
	}
}

func WithBackoffThreshold(n int) cfgFunc {
	return func(p *HttpProxy) {
		p.bkoff_n = n
	}
}

func (p *HttpProxy) Config(funcs ...cfgFunc) {
	for _, f := range funcs {
		f(p)
	}
}

func (p *HttpProxy) setAuth(req *http.Request) error {
	req.SetBasicAuth(p.user, p.passwd)
	auth := req.Header.Get("Authorization")
	req.Header.Set("Proxy-Authorization", auth)
	req.Header.Del("Authorization")
	return nil
}

func (p *HttpProxy) cancelWhenTimeout(cancel context.CancelFunc, stop <-chan struct{}) {
	select {
	case <-time.After(p.timeout):
		cancel()
		p.backoff_hint()
	case <-stop:
		return
	}
}

func (p *HttpProxy) backoff_hint() {
	p.bkoff_ch <- struct{}{}
}

func (p *HttpProxy) backoff_watchdog() {
	count := 0
	tick := time.NewTicker(time.Minute)
	for {
		select {
		case <-p.bkoff_ch:
			count += 1
			if count >= p.bkoff_n {
				p.backoff = true
				count = 0
				<-tick.C // skip this tick
			}
		case <-tick.C:
			p.backoff = false
			count = 0
		}
	}
}

func (p *HttpProxy) DialTunnel(targetUrl string) (*HttpTunnel, error) {
	if p.backoff {
		return nil, errors.New("Backoff")
	}
	// Preamble CONNECT request
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	stopTimeout := make(chan struct{})
	// The cancelWhenTimeout goroutine is used to timeout the Preamble CONNECT req,
	// but we don't want to create any timeout for the tunnel. That's why we don't pass
	// a context.WithTimeout ctx to the NewRequestWithContext.
	go p.cancelWhenTimeout(func() { pw.Close(); cancel() }, stopTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, targetUrl, pr)
	if err != nil {
		close(stopTimeout)
		return nil, err
	}
	p.setAuth(req)
	resp, err := p.httpc.Do(req)
	close(stopTimeout)
	defer func() {
		if err != nil && resp != nil {
			ioutil.ReadAll(resp.Body)
			resp.Body.Close()
		}
	}()
	if err != nil {
		p.backoff_hint()
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

func (p *HttpProxy) Ready() bool {
	return !p.backoff
}

func (p *HttpProxy) Name() string {
	return p.proxyAddr
}

var _ ProxyProvider = &HttpProxy{}
