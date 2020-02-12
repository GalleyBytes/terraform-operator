package proxy

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	socks5 "github.com/cloudfoundry/go-socks5"

	"log"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
)

var netListen = net.Listen

type hostKey interface {
	Get(username, serverURL string, auth ssh.AuthMethod) (ssh.PublicKey, error)
}

type DialFunc func(network, address string) (net.Conn, error)

type Socks5Proxy struct {
	hostKey           hostKey
	port              int
	started           bool
	keepAliveInterval time.Duration
	logger            *log.Logger
	mtx               sync.Mutex
}

func NewSocks5Proxy(hostKey hostKey, logger *log.Logger, keepAliveInterval time.Duration) *Socks5Proxy {
	return &Socks5Proxy{
		hostKey:           hostKey,
		started:           false,
		logger:            logger,
		keepAliveInterval: keepAliveInterval,
	}
}

func (s *Socks5Proxy) Start(username, url string, auth ssh.AuthMethod) error {
	if s.isStarted() {
		return nil
	}

	dialer, err := s.Dialer(username, url, auth)
	if err != nil {
		return err
	}

	err = s.StartWithDialer(dialer)
	if err != nil {
		return err
	}

	return nil
}

// thread safety
func (s *Socks5Proxy) isStarted() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.started
}

func (s *Socks5Proxy) Dialer(username, url string, auth ssh.AuthMethod) (DialFunc, error) {
	if username == "" {
		username = "jumpbox"
	}

	hostKey, err := s.hostKey.Get(username, url, auth)
	if err != nil {
		return nil, fmt.Errorf("get host key: %s", err)
	}

	clientConfig := NewSSHClientConfig(username, ssh.FixedHostKey(hostKey), auth)

	conn, err := ssh.Dial("tcp", url, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %s", err)
	}

	errChan := make(chan error)

	go func(cl *ssh.Client, errChan chan error) {
		t := time.NewTicker(s.keepAliveInterval)
		for {
			select {
			case <-t.C:
				_, _, err := cl.SendRequest("bosh-cli-keep-alive@bosh.io", true, nil)
				if err != nil {
					errChan <- err
				}
			}
		}
	}(conn, errChan)

	go func(errChan chan error) {
		for {
			select {
			case err := <-errChan:
				s.logger.Printf("error sending ssh keep-alive: %s", err)
			}
		}
	}(errChan)

	return conn.Dial, nil
}

func (s *Socks5Proxy) StartWithDialer(dialer DialFunc) error {
	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer(network, addr)
		},
		Logger: s.logger,
	}

	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("new socks5 server: %s", err) // not tested
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.port == 0 {
		s.port, err = openPort()
		if err != nil {
			return fmt.Errorf("open port: %s", err)
		}
	}

	go func() {
		server.ListenAndServe("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	}()

	s.started = true
	return nil
}

func (s *Socks5Proxy) Addr() (string, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.port == 0 {
		return "", errors.New("socks5 proxy is not running")
	}
	return fmt.Sprintf("127.0.0.1:%d", s.port), nil
}

func openPort() (int, error) {
	l, err := netListen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(port)
}
