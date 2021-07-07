package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ruoshan/origdst"
)

var (
	// The following three vars might be injected when by `go link` when building with `--ldflags "-X main.user=abc"`
	user      = "unknown"
	passwd    = "unknown"
	proxyAddr = "unknown"

	localAddr string
	showDebug bool
	timeout   int
	backoff   int
)

var pendingR uint32
var pendingW uint32
var pendingC uint32

func forward(downstream *net.TCPConn, upstream ReadWriteHalfCloser) {
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() { // upstream => downstream
		atomic.AddUint32(&pendingR, 1)
		io.Copy(downstream, upstream)
		upstream.CloseRead()
		downstream.CloseWrite()
		wg.Done()
		atomic.AddUint32(&pendingR, ^uint32(0)) // decreased by 1
	}()
	go func() { // downstream => upstream
		atomic.AddUint32(&pendingW, 1)
		io.Copy(upstream, downstream)
		downstream.CloseRead()
		upstream.CloseWrite()
		wg.Done()
		atomic.AddUint32(&pendingW, ^uint32(0)) // decreased by 1
	}()
	wg.Wait()
}

func sigHandler(f func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	f()
}

func parseArgs() {
	flag.Func("r", "remote proxy address (host:port)", func(s string) error {
		proxyAddr = s
		return nil
	})
	flag.Func("u", "user name", func(s string) error {
		user = s
		return nil
	})
	flag.Func("p", "password", func(s string) error {
		passwd = s
		return nil
	})
	flag.IntVar(&timeout, "t", 10, "CONNECT req timeout (second)")
	flag.IntVar(&backoff, "b", 3, "number of req timeout to trigger backoff")
	flag.StringVar(&localAddr, "l", ":1086", "local addr to bind")
	flag.BoolVar(&showDebug, "d", false, "show debug log")
	flag.Parse()
}

func debug(fmt string, args ...interface{}) {
	if showDebug {
		log.Printf(fmt, args...)
	}
}

func main() {
	parseArgs()

	addr, _ := net.ResolveTCPAddr("tcp", localAddr)
	l, _ := net.ListenTCP("tcp", addr)
	go sigHandler(func() {
		l.Close()
	})

	proxy := NewHttpProxy(proxyAddr, user, passwd)
	proxy.Config(
		WithTimeout(timeout),
		WithBackoffThreshold(backoff),
	)

	// Dump running goroutine count
	go func() {
		if !showDebug {
			return
		}
		tick := time.NewTicker(5 * time.Second)
		for {
			<-tick.C
			debug("W=%d, R=%d, C=%d", pendingW, pendingR, pendingC)
		}
	}()

	// pprof
	go func() {
		http.HandleFunc("/pending", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(fmt.Sprintf("W=%d, R=%d, C=%d\n", pendingW, pendingR, pendingC)))
		})
		http.ListenAndServe(":6060", nil)
	}()

	for {
		c, err := l.AcceptTCP()
		if err != nil {
			return
		}
		go func(c *net.TCPConn) {
			atomic.AddUint32(&pendingC, 1)
			defer atomic.AddUint32(&pendingC, ^uint32(0)) // decreased by 1
			defer c.Close()

			a, err := origdst.GetOriginalDst(c)
			if err != nil {
				debug("Failed to get origdst: %s", err)
				return
			}
			debug("O %s => %s", c.RemoteAddr(), a)
			tunnel, err := proxy.DialTunnel("https://" + a.String())
			if err != nil {
				debug("Failed to tunnel: %s", err)
				return
			}
			forward(c, tunnel)
			debug("X %s => %s", c.RemoteAddr(), a)
		}(c)
	}
}
