package main

import (
	"flag"
	"io"
	"log"
	"net"
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
)

var pending uint32

func forward(downstream *net.TCPConn, upstream ReadWriteHalfCloser) {
	wg := sync.WaitGroup{}
	wg.Add(2)
	atomic.AddUint32(&pending, 1)
	go func() { // upstream => downstream
		io.Copy(downstream, upstream)
		upstream.CloseRead()
		downstream.CloseWrite()
		wg.Done()
		debug("X upstream => downstream")
	}()
	go func() { // downstream => upstream
		io.Copy(upstream, downstream)
		downstream.CloseRead()
		upstream.CloseWrite()
		wg.Done()
		debug("X downstream => upstream")
	}()
	wg.Wait()
	atomic.AddUint32(&pending, ^uint32(0)) // decreased by 1
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

	go func() {
		tick := time.NewTicker(5 * time.Second)
		for {
			<-tick.C
			debug("Pending: %d", pending)
		}
	}()

	for {
		c, err := l.AcceptTCP()
		if err != nil {
			return
		}
		go func(c *net.TCPConn) {
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
