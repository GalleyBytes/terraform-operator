package proxy

import (
	"net"
	"golang.org/x/crypto/ssh"
)

type HostKey struct{}

func NewHostKey() HostKey {
	return HostKey{}
}

func (h HostKey) Get(username, serverURL string, auth ssh.AuthMethod) (ssh.PublicKey, error) {
	publicKeyChannel := make(chan ssh.PublicKey, 1)
	dialErrorChannel := make(chan error)

	if username == "" {
		username = "jumpbox"
	}

	clientConfig := NewSSHClientConfig(username, keyScanCallback(publicKeyChannel), auth)

	go func() {
		conn, err := ssh.Dial("tcp", serverURL, clientConfig)
		if err != nil {
			publicKeyChannel <- nil
			dialErrorChannel <- err
			return
		}
		defer conn.Close()
		dialErrorChannel <- nil
	}()

	return <-publicKeyChannel, <-dialErrorChannel
}

func keyScanCallback(publicKeyChannel chan ssh.PublicKey) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		publicKeyChannel <- key
		return nil
	}
}
