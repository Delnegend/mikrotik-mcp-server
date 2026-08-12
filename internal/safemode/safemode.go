// Package safemode implements RouterOS Safe Mode over a persistent SSH
// console session. While safe mode is active, changes made through the session
// are held in memory only; a deliberate commit persists them, and dropping the
// session rolls every pending change back automatically.
//
// RouterOS Safe Mode is a terminal-session feature: only commands typed in the
// session that enabled it are protected. The server therefore routes mutating
// tool calls through this CLI session while safe mode is active.
package safemode

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
	"github.com/Delnegend/mikrotik-mcp/internal/inventory"
	"golang.org/x/crypto/ssh"
)

const (
	ctrlX   = byte(0x18)
	timeout = 15 * time.Second
)

var (
	// The RouterOS console echoes over the pty with CR/LF interleaved between
	// characters and ANSI escapes sprinkled in, so prompts are matched on the
	// ANSI-stripped stream with optional CR/LF between significant characters.
	// The trailing ">" of the <SAFE> prompt is observed to be dropped by the
	// mangled echo, so it is optional.
	reSafePrompt = regexp.MustCompile(`\[[^\]]*\][ \t\r\n]*<[ \t\r\n]*S[ \t\r\n]*A[ \t\r\n]*F[ \t\r\n]*E[ \t\r\n]*>[ \t\r\n]*(?:>[ \t\r\n]*)?$`)
	rePrompt     = regexp.MustCompile(`\[[^\]]*\][ \t\r\n]*>[ \t\r\n]*$`)
	reAnyPrompt  = regexp.MustCompile(`\[[^\]]*\][ \t\r\n]*(?:<[ \t\r\n]*S[ \t\r\n]*A[ \t\r\n]*F[ \t\r\n]*E[ \t\r\n]*>[ \t\r\n]*)?(?:>[ \t\r\n]*)?$`)
	// First-console-login license question on fresh installs.
	reLicense = regexp.MustCompile(`\[Y/n\]:[ \t]*$`)
	// ANSI/8-bit CSI escape sequences emitted by the console.
	reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x9b[0-9;?]*[ -/]*[@-~]|\x1bZ|\x1b\[c`)
)

// BuildCLI renders a RouterOS CLI command from a command path and attributes,
// with keys sorted for deterministic output and values containing whitespace
// quoted with RouterOS double-quote semantics.
func BuildCLI(command string, attrs map[string]any) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, command)
	for _, k := range keys {
		if attrs[k] == nil {
			continue
		}
		parts = append(parts, k+"="+quoteCLIValue(fmt.Sprint(attrs[k])))
	}
	return strings.Join(parts, " ")
}

func quoteCLIValue(v string) string {
	if strings.ContainsAny(v, " \t\n\r\"") {
		v = strings.ReplaceAll(v, `"`, `\"`)
		return `"` + v + `"`
	}
	return v
}

// findPromptEnd reports the index at which the prompt starts, or -1 when the
// buffer does not end with the prompt. Only occurrences at the end of the
// buffer (ignoring trailing CR/LF) are accepted, so prompts echoed in command
// output do not match.
func findPromptEnd(buf []byte, re *regexp.Regexp) int {
	loc := re.FindIndex(buf)
	if loc == nil {
		return -1
	}
	for _, b := range buf[loc[1]:] {
		if b != '\r' && b != '\n' && b != ' ' {
			return -1
		}
	}
	return loc[0]
}

// Session is one persistent RouterOS SSH console session.
type Session struct {
	sshClient *ssh.Client
	sshSess   *ssh.Session
	stdin     io.WriteCloser
	stdout    io.Reader
	pending   []byte
	active    bool
	cprCount  int // terminal query responses sent
	daCount   int
}

// Enable opens an SSH console session and activates RouterOS Safe Mode.
func Enable(dev inventory.Device) (*Session, error) {
	cfg, err := sshConfig(dev)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(dev.Host, strconv.Itoa(dev.SSHPort))
	sshClient, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("safe mode: SSH connect to %s: %v", addr, err)
	}
	sess, err := sshClient.NewSession()
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("safe mode: open session: %v", err)
	}
	sess.Stderr = nil

	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		sshClient.Close()
		return nil, fmt.Errorf("safe mode: stdin: %v", err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		sshClient.Close()
		return nil, fmt.Errorf("safe mode: stdout: %v", err)
	}
	// RouterOS presents the interactive console only on a terminal.
	if err := sess.RequestPty("xterm", 132, 43, ssh.TerminalModes{
		ssh.ECHO: 1, ssh.OPOST: 1, ssh.ONLCR: 1,
	}); err != nil {
		sess.Close()
		sshClient.Close()
		return nil, fmt.Errorf("safe mode: request pty: %v", err)
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		sshClient.Close()
		return nil, fmt.Errorf("safe mode: start shell: %v", err)
	}

	s := &Session{
		sshClient: sshClient,
		sshSess:   sess,
		stdin:     stdin,
		stdout:    stdout,
	}

	// Wait for the console to be ready (declining the first-login software
	// license question if asked), then activate safe mode and wait for the
	// <SAFE> prompt.
	if err := s.readyConsole(); err != nil {
		s.Close()
		return nil, fmt.Errorf("safe mode: %v", err)
	}
	if _, err := s.writeAndWait([]byte{ctrlX}, reSafePrompt); err != nil {
		s.Close()
		return nil, fmt.Errorf("safe mode: router did not enter safe mode: %v (output: %q)", err, string(s.pending))
	}
	s.active = true
	return s, nil
}

// readyConsole reads until the router shows a shell prompt, declining the
// one-time software-license question when a fresh install asks for it.
func (s *Session) readyConsole() error {
	deadline := time.Now().Add(timeout)
	for {
		if idx := findPromptEnd(s.pending, reAnyPrompt); idx >= 0 {
			s.pending = s.pending[idx:]
			return nil
		}
		if reLicense.Match(s.pending) {
			s.pending = nil
			if _, err := s.stdin.Write([]byte("n\n")); err != nil {
				return err
			}
			continue
		}
		buf := make([]byte, 4096)
		n, err := readBounded(s.stdout, buf, deadline)
		if err != nil {
			return fmt.Errorf("waiting for console prompt: %v (output so far: %q)", err, string(s.pending))
		}
		s.pending = append(s.pending, buf[:n]...)
		s.respondToTerminalQueries()
		s.pending = reANSI.ReplaceAll(s.pending, nil)
	}
}

// Active reports whether safe mode is on in this session.
func (s *Session) Active() bool { return s.active }

// Execute runs one CLI command through the session and returns its output.
// While safe mode is active the prompt carries <SAFE>, so either prompt ends
// the read.
func (s *Session) Execute(cmd string) (string, error) {
	return s.writeAndWait([]byte(cmd+"\n"), reAnyPrompt)
}

// Commit persists the pending changes and leaves safe mode.
func (s *Session) Commit() error {
	if !s.active {
		return errors.New("safe mode is not active")
	}
	if _, err := s.writeAndWait([]byte{ctrlX}, rePrompt); err != nil {
		return fmt.Errorf("safe mode: commit failed: %v", err)
	}
	s.active = false
	return s.Close()
}

// Rollback discards pending changes by dropping the session; RouterOS reverts
// everything automatically on disconnect.
func (s *Session) Rollback() error {
	if !s.active {
		return errors.New("safe mode is not active")
	}
	s.active = false
	return s.Close()
}

// Add runs a CLI add on the given menu through the safe-mode session.
func (s *Session) Add(menu string, attrs map[string]any) (map[string]any, error) {
	if _, err := s.Execute(BuildCLI(menu+"/add", attrs)); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// Set runs a CLI set through the safe-mode session.
func (s *Session) Set(menu, itemID string, attrs map[string]any) (map[string]any, error) {
	m := make(map[string]any, len(attrs)+1)
	maps.Copy(m, attrs)
	m[".id"] = itemID
	if _, err := s.Execute(BuildCLI(menu+"/set", m)); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// Remove runs a CLI remove through the safe-mode session.
func (s *Session) Remove(menu, itemID string) (map[string]any, error) {
	if _, err := s.Execute(BuildCLI(menu+"/remove", map[string]any{".id": itemID})); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// Run executes an arbitrary CLI command through the safe-mode session.
func (s *Session) Run(path string, attrs map[string]any) (map[string]any, error) {
	if _, err := s.Execute(BuildCLI(path, attrs)); err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

// Close tears down the session.
func (s *Session) Close() error {
	var first error
	if s.sshSess != nil {
		if err := s.sshSess.Close(); err != nil && first == nil {
			first = err
		}
	}
	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Session) writeAndWait(data []byte, prompt *regexp.Regexp) (string, error) {
	if _, err := s.stdin.Write(data); err != nil {
		return "", err
	}
	return s.waitForPrompt(prompt)
}

func (s *Session) waitForPrompt(prompt *regexp.Regexp) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		if idx := findPromptEnd(s.pending, prompt); idx >= 0 {
			out := string(s.pending[:idx])
			s.pending = s.pending[idx:]
			return strings.TrimSpace(out), nil
		}
		buf := make([]byte, 4096)
		n, err := readBounded(s.stdout, buf, deadline)
		if err != nil {
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout waiting for RouterOS prompt; output so far: %q", string(s.pending))
			}
			return "", err
		}
		s.pending = append(s.pending, buf[:n]...)
		s.respondToTerminalQueries()
		s.pending = reANSI.ReplaceAll(s.pending, nil)
	}
}

var (
	reCursorPos = []byte("\x1b[6n") // cursor position request
	reDevAttrs  = []byte("\x1bZ")   // device attributes request
	reDevAttrs2 = []byte("\x1b[c")  // device attributes request (variant)
)

// respondToTerminalQueries answers RouterOS's in-band terminal queries. The
// console performs terminal detection during setup and after toggling safe
// mode; without the responses it can stall or drop the session (SIGPIPE).
func (s *Session) respondToTerminalQueries() {
	cpr := bytes.Count(s.pending, reCursorPos)
	for ; s.cprCount < cpr; s.cprCount++ {
		_, _ = s.stdin.Write([]byte("\x1b[1;1R"))
	}
	da := bytes.Count(s.pending, reDevAttrs) + bytes.Count(s.pending, reDevAttrs2)
	for ; s.daCount < da; s.daCount++ {
		_, _ = s.stdin.Write([]byte("\x1b[?1;2c"))
	}
}

// readBounded reads from r, aborting when the deadline passes so a silent
// remote end can never block a session forever.
func readBounded(r io.Reader, buf []byte, deadline time.Time) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := r.Read(buf)
		ch <- result{n, err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(time.Until(deadline)):
		return 0, errors.New("read timed out")
	}
}

func sshConfig(dev inventory.Device) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:    dev.SSHUsername,
		Timeout: 10 * time.Second,
	}
	if dev.Password != "" {
		cfg.Auth = []ssh.AuthMethod{ssh.Password(dev.Password)}
	}
	if dev.SSHFingerprintSHA256 != "" {
		cfg.HostKeyCallback = fingerprintPolicy(dev.Host, dev.SSHFingerprintSHA256)
	} else if helpers.ParseBool(os.Getenv("MIKROTIK_SCP_INSECURE"), false) {
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		return nil, errors.New("safe mode requires SSH host key verification: set MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 or MIKROTIK_SCP_INSECURE=1")
	}
	return cfg, nil
}

func fingerprintPolicy(hostname, expected string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if actual := ssh.FingerprintSHA256(key); actual != expected {
			return fmt.Errorf("SSH host key fingerprint mismatch for %s: expected %s, got %s",
				hostname, expected, actual)
		}
		return nil
	}
}

// Manager tracks one safe-mode session per device.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

func (m *Manager) Session(title string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[title]
}

func (m *Manager) Active(title string) bool {
	s := m.Session(title)
	return s != nil && s.Active()
}

func (m *Manager) Enable(title string, dev inventory.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[title]; ok && s.Active() {
		return fmt.Errorf("safe mode is already active for %q", title)
	}
	s, err := Enable(dev)
	if err != nil {
		return err
	}
	m.sessions[title] = s
	return nil
}

func (m *Manager) Commit(title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[title]
	if !ok || !s.Active() {
		return fmt.Errorf("safe mode is not active for %q", title)
	}
	err := s.Commit()
	if err != nil {
		return err
	}
	delete(m.sessions, title)
	return nil
}

func (m *Manager) Rollback(title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[title]
	if !ok || !s.Active() {
		return fmt.Errorf("safe mode is not active for %q (sessions=%d)", title, len(m.sessions))
	}
	_ = s.Rollback()
	delete(m.sessions, title)
	return nil
}
