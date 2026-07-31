package downloads

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

type inMemorySSHServer struct {
	t          *testing.T
	hostKey    ssh.Signer
	password   string
	commands   []string
	mu         sync.Mutex
	listener   net.Listener
	listenAddr string
}

func newInMemorySSHServer(t *testing.T) *inMemorySSHServer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := &inMemorySSHServer{
		t:          t,
		hostKey:    signer,
		listener:   lis,
		listenAddr: lis.Addr().String(),
	}
	go srv.acceptLoop()
	return srv
}

func (s *inMemorySSHServer) addr() string {
	return s.listenAddr
}

func (s *inMemorySSHServer) close() {
	s.listener.Close()
}

func (s *inMemorySSHServer) fingerprint() string {
	return ssh.FingerprintSHA256(s.hostKey.PublicKey())
}

func (s *inMemorySSHServer) setPassword(pw string) {
	s.password = pw
}

func (s *inMemorySSHServer) commandsRun() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.commands))
	copy(result, s.commands)
	return result
}

func (s *inMemorySSHServer) serverConfig() *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if s.password == "" || string(pass) == s.password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil
		},
		NoClientAuth: s.password == "",
	}
	config.AddHostKey(s.hostKey)
	return config
}

func (s *inMemorySSHServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *inMemorySSHServer) handleConn(tcpConn net.Conn) {
	_, chans, reqs, err := ssh.NewServerConn(tcpConn, s.serverConfig())
	if err != nil {
		s.t.Logf("server conn error: %v", err)
		tcpConn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	s.handleChannels(chans)
}

func (s *inMemorySSHServer) handleChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, reqs, err := ch.Accept()
		if err != nil {
			s.t.Logf("accept channel error: %v", err)
			continue
		}
		go func() {
			defer channel.Close()
			for req := range reqs {
				switch req.Type {
				case "exec":
					s.mu.Lock()
					s.commands = append(s.commands, string(req.Payload))
					s.mu.Unlock()
					req.Reply(true, nil)
					channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					return
				case "subsystem":
					req.Reply(false, nil)
				default:
					req.Reply(true, nil)
				}
			}
		}()
	}
}
