package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

var (
	ErrRouterOSFatalError     = errors.New("routeros: fatal error")
	ErrRouterOSAuthError      = errors.New("routeros: auth error")
	ErrRouterOSTransportError = errors.New("routeros: transport error")
)

// RouterOSError wraps RouterOS command errors with message and optional category.
type RouterOSError struct {
	Message  string
	Category string
}

func (e *RouterOSError) Error() string {
	if e.Category != "" {
		return fmt.Sprintf("RouterOS command failed (%s): %s", e.Category, e.Message)
	}
	return fmt.Sprintf("RouterOS command failed: %s", e.Message)
}

// ReplyBundle holds all reply sentences from a single command execution.
type ReplyBundle struct {
	Records []map[string]string
	Done    map[string]string
	Traps   []map[string]string
	Fatal   map[string]string
	Empty   bool
	Tag     string
}

// ListenResult holds the result of a bounded /listen subscription.
type ListenResult struct {
	*ReplyBundle
	Cancelled    bool
	LimitReached bool
	CancelDone   map[string]string
	CancelFatal  map[string]string
}

// ---------------------------------------------------------------------------
// Wire protocol: variable-length word encoding
// ---------------------------------------------------------------------------

func encodeLength(length int) ([]byte, error) {
	if length < 0 {
		return nil, errors.New("word length must be non-negative")
	}
	if length < 0x80 {
		return []byte{byte(length)}, nil
	}
	if length < 0x4000 {
		v := length | 0x8000
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		return b, nil
	}
	if length < 0x200000 {
		v := uint32(length | 0xC00000)
		b4 := make([]byte, 4)
		binary.BigEndian.PutUint32(b4, v)
		return b4[1:], nil // 3 bytes
	}
	if length < 0x10000000 {
		v := uint32(length | 0xE0000000)
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, v)
		return b, nil
	}
	if length < 0x100000000 {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(length))
		return append([]byte{0xF0}, b...), nil
	}
	return nil, errors.New("word length exceeds RouterOS API limit")
}

func decodeLength(r io.Reader) (int, error) {
	first := make([]byte, 1)
	if _, err := io.ReadFull(r, first); err != nil {
		return 0, err
	}
	v := first[0]

	switch {
	case (v & 0x80) == 0x00:
		return int(v), nil
	case (v & 0xC0) == 0x80:
		rest := make([]byte, 1)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, err
		}
		return int(uint16(v&0x3F)<<8 | uint16(rest[0])), nil
	case (v & 0xE0) == 0xC0:
		rest := make([]byte, 2)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, err
		}
		return int(uint32(v&0x1F)<<16 | uint32(rest[0])<<8 | uint32(rest[1])), nil
	case (v & 0xF0) == 0xE0:
		rest := make([]byte, 3)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, err
		}
		return int(uint32(v&0x0F)<<24 | uint32(rest[0])<<16 | uint32(rest[1])<<8 | uint32(rest[2])), nil
	case v == 0xF0:
		rest := make([]byte, 4)
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, err
		}
		return int(binary.BigEndian.Uint32(rest)), nil
	default:
		return 0, fmt.Errorf("unsupported RouterOS length prefix: 0x%02x", v)
	}
}

func encodeSentence(words []string) []byte {
	var buf bytes.Buffer
	for _, word := range words {
		data := []byte(word)
		lenBytes, _ := encodeLength(len(data))
		buf.Write(lenBytes)
		buf.Write(data)
	}
	buf.WriteByte(0)
	return buf.Bytes()
}

func readWord(r io.Reader) (string, error) {
	length, err := decodeLength(r)
	if err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	// Try UTF-8 first; fall back to latin-1 for file metadata
	if utf8.Valid(data) {
		return string(data), nil
	}
	// Fallback: interpret as latin-1
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes), nil
}

func readSentence(r io.Reader) ([]string, error) {
	var words []string
	for {
		word, err := readWord(r)
		if err != nil {
			return nil, err
		}
		if word == "" {
			return words, nil
		}
		words = append(words, word)
	}
}

// ---------------------------------------------------------------------------
// Reply parsing
// ---------------------------------------------------------------------------

func parseReplySentence(sentence []string) (string, map[string]string) {
	if len(sentence) == 0 {
		panic("sentence must not be empty")
	}
	replyType := sentence[0]
	attrs := make(map[string]string)
	for _, word := range sentence[1:] {
		if word == "" {
			continue
		}
		if strings.HasPrefix(word, ".") && strings.Contains(word, "=") {
			rest := word[1:]
			key, value, found := strings.Cut(rest, "=")
			if found {
				attrs["."+key] = value
			}
		} else if strings.HasPrefix(word, "=") {
			rest := word[1:]
			key, value, found := strings.Cut(rest, "=")
			if found {
				attrs[key] = value
			}
		} else {
			attrs[word] = ""
		}
	}
	return replyType, attrs
}

func parseReplySentences(sentences [][]string) *ReplyBundle {
	bundle := &ReplyBundle{}
	for _, sentence := range sentences {
		if len(sentence) == 0 {
			continue
		}
		replyType, attrs := parseReplySentence(sentence)
		if bundle.Tag == "" {
			bundle.Tag = attrs[".tag"]
		}
		switch replyType {
		case "!re":
			bundle.Records = append(bundle.Records, attrs)
		case "!done":
			bundle.Done = attrs
		case "!trap":
			bundle.Traps = append(bundle.Traps, attrs)
		case "!fatal":
			bundle.Fatal = attrs
		case "!empty":
			bundle.Empty = true
		}
	}
	return bundle
}

// ---------------------------------------------------------------------------
// RouterOSClient
// ---------------------------------------------------------------------------

type RouterOSClient struct {
	host       string
	username   string
	password   string
	port       int
	useSSL     bool
	tlsVerify  bool
	tlsCAFiles []string
	timeout    time.Duration
	conn       net.Conn
	br         *bufio.Reader
	certPool   *x509.CertPool
	mu         sync.Mutex
	execMu     sync.Mutex
}

func NewRouterOSClient(host, username, password string, opts ...ClientOption) *RouterOSClient {
	c := &RouterOSClient{
		host:     host,
		username: username,
		password: password,
		port:     8728,
		timeout:  10 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	if c.useSSL && c.port == 8728 {
		c.port = 8729
	}
	return c
}

type ClientOption func(*RouterOSClient)

func WithPort(port int) ClientOption {
	return func(c *RouterOSClient) { c.port = port }
}

func WithTLS(enabled bool) ClientOption {
	return func(c *RouterOSClient) { c.useSSL = enabled }
}

func WithTLSVerify(verify bool) ClientOption {
	return func(c *RouterOSClient) { c.tlsVerify = verify }
}

func WithTLSCAFiles(files []string) ClientOption {
	return func(c *RouterOSClient) { c.tlsCAFiles = files }
}

func WithTimeout(d time.Duration) ClientOption {
	return func(c *RouterOSClient) { c.timeout = d }
}

func (c *RouterOSClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	rawConn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return fmt.Errorf("%w: failed to connect to %s: %v", ErrRouterOSTransportError, addr, err)
	}

	if c.useSSL {
		tlsConfig := &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: !c.tlsVerify,
		}
		if c.tlsVerify && len(c.tlsCAFiles) > 0 {
			certPool, err := c.loadCertPool()
			if err != nil {
				rawConn.Close()
				return fmt.Errorf("%w: %v", ErrRouterOSTransportError, err)
			}
			tlsConfig.RootCAs = certPool
		}

		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			if c.tlsVerify {
				return fmt.Errorf("%w: TLS connection to %s failed. Place trusted CA certs in certs/ or set MIKROTIK_TLS_VERIFY=false for self-signed lab certs. %v",
					ErrRouterOSTransportError, addr, err)
			}
			return fmt.Errorf("%w: TLS connection to %s failed: %v", ErrRouterOSTransportError, addr, err)
		}
		c.conn = tlsConn
	} else {
		c.conn = rawConn
	}
	c.br = bufio.NewReader(c.conn)
	return nil
}

func (c *RouterOSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.br = nil
		return err
	}
	return nil
}

func (c *RouterOSClient) Open() error {
	if err := c.Connect(); err != nil {
		return err
	}
	return c.Login()
}

func (c *RouterOSClient) Clone() *RouterOSClient {
	return NewRouterOSClient(c.host, c.username, c.password,
		WithPort(c.port),
		WithTLS(c.useSSL),
		WithTLSVerify(c.tlsVerify),
		WithTLSCAFiles(c.tlsCAFiles),
		WithTimeout(c.timeout),
	)
}

func (c *RouterOSClient) Isolated(fn func(client *RouterOSClient) error) error {
	cloned := c.Clone()
	if err := cloned.Open(); err != nil {
		return err
	}
	defer cloned.Close()
	return fn(cloned)
}

func (c *RouterOSClient) Login() error {
	c.execMu.Lock()
	defer c.execMu.Unlock()
	return c.loginLocked()
}

// loginLocked runs the /login exchange; the caller must hold execMu.
func (c *RouterOSClient) loginLocked() error {
	words, err := buildCommandSentence("/login",
		map[string]any{"name": c.username, "password": c.password}, nil, "")
	if err != nil {
		return err
	}
	reply, err := c.executeLocked(words)
	if err != nil {
		return err
	}
	if len(reply.Traps) > 0 {
		trap := reply.Traps[0]
		message := trap["message"]
		if message == "" {
			message = "Login failed"
		}
		return fmt.Errorf("%w: RouterOS login failed for user '%s': %s", ErrRouterOSAuthError, c.username, message)
	}
	if reply.Fatal != nil {
		msg := reply.Fatal["message"]
		if msg == "" {
			msg = "RouterOS connection ended during login"
		}
		return fmt.Errorf("%w: %s", ErrRouterOSFatalError, msg)
	}
	return nil
}

func (c *RouterOSClient) Print(menu string, proplist, queries []string, attrs map[string]any) ([]map[string]string, error) {
	normalizedMenu, err := normalizeMenu(menu)
	if err != nil {
		return nil, err
	}
	words := []string{normalizedMenu + "/print"}
	if len(proplist) > 0 {
		words = append(words, "=.proplist="+strings.Join(proplist, ","))
	}
	for _, attr := range normalizeAttrs(attrs) {
		words = append(words, "="+attr.key+"="+attr.value)
	}
	normalizedQueries, err := normalizeQueries(queries)
	if err != nil {
		return nil, err
	}
	for _, query := range normalizedQueries {
		words = append(words, query)
	}

	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	if err := raiseForErrors(reply); err != nil {
		return nil, err
	}
	return reply.Records, nil
}

func (c *RouterOSClient) Add(menu string, attrs map[string]any) (map[string]any, error) {
	words, err := buildMenuSentence(menu, "add", "", attrs)
	if err != nil {
		return nil, err
	}
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Set(menu, itemID string, attrs map[string]any) (map[string]any, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, errors.New("item_id is required")
	}
	words, err := buildMenuSentence(menu, "set", itemID, attrs)
	if err != nil {
		return nil, err
	}
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Remove(menu, itemID string) (map[string]any, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, errors.New("item_id is required")
	}
	words, err := buildMenuSentence(menu, "remove", itemID, nil)
	if err != nil {
		return nil, err
	}
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Run(path string, attrs map[string]any, queries []string, tag string) (any, error) {
	words, err := buildCommandSentence(path, attrs, queries, tag)
	if err != nil {
		return nil, err
	}
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	if err := raiseForErrors(reply); err != nil {
		return nil, err
	}
	if len(reply.Records) > 0 {
		return reply.Records, nil
	}
	return normalizeMutationResult(reply)
}

// RunContext is Run with cancellation support: when ctx is cancelled the
// tagged command is interrupted with /cancel and the error is ctx.Err().
func (c *RouterOSClient) RunContext(ctx context.Context, path string, attrs map[string]any, queries []string, tag string) (any, error) {
	words, err := buildCommandSentence(path, attrs, queries, tag)
	if err != nil {
		return nil, err
	}
	reply, err := c.executeContext(ctx, words)
	if err != nil {
		return nil, err
	}
	if err := raiseForErrors(reply); err != nil {
		return nil, err
	}
	if len(reply.Records) > 0 {
		return reply.Records, nil
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Listen(menu string, proplist, queries []string, attrs map[string]any, tag string, maxEvents int) (*ListenResult, error) {
	return c.ListenContext(context.Background(), menu, proplist, queries, attrs, tag, maxEvents)
}

// ListenContext is Listen with cancellation support: when ctx is cancelled the
// listen is interrupted with /cancel and the result is marked Cancelled.
func (c *RouterOSClient) ListenContext(ctx context.Context, menu string, proplist, queries []string, attrs map[string]any, tag string, maxEvents int) (*ListenResult, error) {
	c.execMu.Lock()
	defer c.execMu.Unlock()
	defer c.clearDeadline()

	if maxEvents < 1 {
		return nil, errors.New("max_events must be at least 1")
	}

	listenTag := tag
	if listenTag == "" {
		listenTag = c.generateTag("listen")
	}
	cancelTag := listenTag + "-cancel"

	normalizedMenu, err := normalizeMenu(menu)
	if err != nil {
		return nil, err
	}
	words := []string{normalizedMenu + "/listen"}
	if len(proplist) > 0 {
		words = append(words, "=.proplist="+strings.Join(proplist, ","))
	}
	for _, attr := range normalizeAttrs(attrs) {
		words = append(words, "="+attr.key+"="+attr.value)
	}
	normalizedQueries, err := normalizeQueries(queries)
	if err != nil {
		return nil, err
	}
	for _, query := range normalizedQueries {
		words = append(words, query)
	}
	words = append(words, ".tag="+listenTag)

	result := &ListenResult{ReplyBundle: &ReplyBundle{Tag: listenTag}}

	c.setDeadline()
	if err := c.writeSentence(words); err != nil {
		return nil, err
	}

	if ctx.Done() != nil {
		watchDone := make(chan struct{})
		defer close(watchDone)
		go func() {
			select {
			case <-ctx.Done():
				_ = c.conn.SetDeadline(time.Now())
			case <-watchDone:
			}
		}()
	}

	cancelSent := false
	listenDone := false
	cancelDone := false

	for {
		if ctx.Err() != nil && !cancelSent {
			result.Cancelled = true
			if writeErr := c.sendCancel(listenTag); writeErr != nil {
				return nil, writeErr
			}
			cancelSent = true
		}
		c.setDeadline()
		sentence, err := readSentence(c.reader())
		if err != nil {
			msg := err.Error()
			// On timeout, send cancel
			if !cancelSent && (errors.Is(err, ErrRouterOSTransportError) || strings.Contains(strings.ToLower(msg), "timeout")) {
				result.Cancelled = true
				c.setDeadline() // refresh the deadline so the cancel write can succeed
				if writeErr := c.sendCancel(listenTag); writeErr != nil {
					return nil, writeErr
				}
				cancelSent = true
				continue
			}
			if ctx.Err() != nil {
				// Interrupted by caller cancellation; keep draining.
				continue
			}
			return nil, err
		}

		replyType, replyAttrs := parseReplySentence(sentence)
		replyTag := replyAttrs[".tag"]

		belongsToListen := replyTag == listenTag || (replyTag == "" && !cancelSent &&
			(replyType == "!trap" || replyType == "!fatal" || replyType == "!done" || replyType == "!empty"))

		if belongsToListen {
			switch replyType {
			case "!re":
				result.Records = append(result.Records, replyAttrs)
				if len(result.Records) >= maxEvents && !cancelSent {
					result.LimitReached = true
					result.Cancelled = true
					if writeErr := c.sendCancel(listenTag); writeErr != nil {
						return nil, writeErr
					}
					cancelSent = true
				}
			case "!trap":
				if cancelSent && replyAttrs["message"] == "interrupted" {
					result.Cancelled = true
				} else {
					result.Traps = append(result.Traps, replyAttrs)
				}
			case "!done":
				result.Done = replyAttrs
				listenDone = true
			case "!fatal":
				result.Fatal = replyAttrs
				listenDone = true
			case "!empty":
				result.Empty = true
			}
		}

		belongsToCancel := replyTag == cancelTag || (cancelSent && replyTag == "" &&
			(replyType == "!done" || replyType == "!fatal"))

		if belongsToCancel {
			switch replyType {
			case "!done":
				result.CancelDone = replyAttrs
				cancelDone = true
			case "!fatal":
				result.CancelFatal = replyAttrs
				cancelDone = true
			}
		}

		if result.CancelFatal != nil || (listenDone && (!cancelSent || cancelDone)) {
			break
		}
	}

	if result.CancelFatal != nil {
		msg := result.CancelFatal["message"]
		if msg == "" {
			msg = "RouterOS cancel command ended unexpectedly"
		}
		return nil, fmt.Errorf("%w: %s", ErrRouterOSFatalError, msg)
	}
	if result.Fatal != nil {
		msg := result.Fatal["message"]
		if msg == "" {
			msg = "RouterOS connection ended unexpectedly"
		}
		return nil, fmt.Errorf("%w: %s", ErrRouterOSFatalError, msg)
	}
	return result, nil
}

func (c *RouterOSClient) Cancel(tag string) (map[string]any, error) {
	if strings.TrimSpace(tag) == "" {
		return nil, errors.New("tag is required")
	}
	words, err := buildCancelSentence(tag, "")
	if err != nil {
		return nil, err
	}
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) execute(words []string) (*ReplyBundle, error) {
	return c.executeContext(context.Background(), words)
}

// executeLocked sends a sentence and reads the full reply; the caller must
// hold execMu. Used by loginLocked so a lazy connect can authenticate without
// re-acquiring the lock it already holds.
func (c *RouterOSClient) executeLocked(words []string) (*ReplyBundle, error) {
	c.setDeadline()
	if err := c.writeSentence(words); err != nil {
		return nil, err
	}
	var sentences [][]string
	for {
		sentence, err := readSentence(c.reader())
		if err != nil {
			return nil, err
		}
		sentences = append(sentences, sentence)
		if len(sentence) > 0 {
			first := sentence[0]
			if first == "!done" || first == "!fatal" {
				break
			}
		}
	}
	return parseReplySentences(sentences), nil
}

func (c *RouterOSClient) executeContext(ctx context.Context, words []string) (*ReplyBundle, error) {
	c.execMu.Lock()
	defer c.execMu.Unlock()
	defer c.clearDeadline()

	if c.conn == nil {
		// Lazy connect under execMu. Connect takes c.mu and loginLocked does
		// not touch execMu, so this cannot deadlock.
		if err := c.Connect(); err != nil {
			return nil, err
		}
		if err := c.loginLocked(); err != nil {
			return nil, err
		}
	}

	tag := ""
	for _, w := range words {
		if rest, ok := strings.CutPrefix(w, ".tag="); ok {
			tag = rest
		}
	}
	if ctx.Done() != nil && tag == "" {
		tag = c.generateTag("ctx")
		words = append(words, ".tag="+tag)
	}

	c.setDeadline()
	if err := c.writeSentence(words); err != nil {
		return nil, err
	}

	if ctx.Done() != nil {
		watchDone := make(chan struct{})
		defer close(watchDone)
		go func() {
			select {
			case <-ctx.Done():
				_ = c.conn.SetDeadline(time.Now())
			case <-watchDone:
			}
		}()
	}

	var sentences [][]string
	for {
		sentence, err := readSentence(c.reader())
		if err != nil {
			if ctx.Err() != nil {
				_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
				if writeErr := c.sendCancel(tag); writeErr != nil {
					return nil, writeErr
				}
				for {
					s, rerr := readSentence(c.reader())
					if rerr != nil {
						return nil, ctx.Err()
					}
					sentences = append(sentences, s)
					if len(s) > 0 && (s[0] == "!done" || s[0] == "!fatal") {
						break
					}
				}
				return nil, ctx.Err()
			}
			return nil, err
		}
		sentences = append(sentences, sentence)
		if len(sentence) > 0 {
			first := sentence[0]
			if first == "!done" || first == "!fatal" {
				break
			}
		}
	}
	return parseReplySentences(sentences), nil
}

func (c *RouterOSClient) writeSentence(words []string) error {
	data := encodeSentence(words)
	if c.conn == nil {
		return fmt.Errorf("%w: RouterOS socket is not connected", ErrRouterOSTransportError)
	}
	_, err := c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("%w: failed to send RouterOS API sentence: %v", ErrRouterOSTransportError, err)
	}
	return nil
}

func (c *RouterOSClient) setDeadline() {
	if c.conn != nil {
		// Deliberately ignored: if the connection is already broken,
		// the subsequent readSentence/writeSentence will surface the
		// real transport error.
		_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
	}
}

func (c *RouterOSClient) clearDeadline() {
	if c.conn != nil {
		// SetDeadline applies to both reads and writes; a deadline left
		// expired after a timeout would otherwise break every later
		// operation on this socket.
		_ = c.conn.SetDeadline(time.Time{})
	}
}

func (c *RouterOSClient) sendCancel(tag string) error {
	words, err := buildCancelSentence(tag, "")
	if err != nil {
		return err
	}
	return c.writeSentence(words)
}

func (c *RouterOSClient) reader() *bufio.Reader {
	if c.br == nil && c.conn != nil {
		c.br = bufio.NewReader(c.conn)
	}
	return c.br
}

func (c *RouterOSClient) loadCertPool() (*x509.CertPool, error) {
	if c.certPool != nil {
		return c.certPool, nil
	}
	pool := x509.NewCertPool()
	for _, f := range c.tlsCAFiles {
		pem, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert %s: %v", f, err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no valid CA cert found in %s", f)
		}
	}
	c.certPool = pool
	return pool, nil
}

var tagCounter atomic.Uint64

func (c *RouterOSClient) generateTag(prefix string) string {
	return prefix + "-" + strconv.FormatUint(tagCounter.Add(1), 16)
}

func (c *RouterOSClient) Host() string { return c.host }
func (c *RouterOSClient) Port() int    { return c.port }
func (c *RouterOSClient) UseSSL() bool { return c.useSSL }

// TLSSessionInfo returns TLS session details for the connection.
func (c *RouterOSClient) TLSSessionInfo() map[string]any {
	tlsConn, ok := c.conn.(*tls.Conn)
	if !ok {
		return nil
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil
	}
	cert := state.PeerCertificates[0]
	der := cert.Raw
	fingerprint := fmt.Sprintf("%X", sha256.Sum256(der))

	info := map[string]any{
		"subject":            cert.Subject.String(),
		"issuer":             cert.Issuer.String(),
		"serial_number":      cert.SerialNumber.String(),
		"not_before":         cert.NotBefore.Format(time.RFC3339),
		"not_after":          cert.NotAfter.Format(time.RFC3339),
		"subject_alt_names":  cert.DNSNames,
		"sha256_fingerprint": fingerprint,
		"tls_version":        tlsVersionName(state.Version),
		"cipher":             tls.CipherSuiteName(state.CipherSuite),
		"hostname_verified":  c.tlsVerify,
	}
	return info
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLSv1.0"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS13:
		return "TLSv1.3"
	default:
		return fmt.Sprintf("TLS-0x%04x", version)
	}
}

// ---------------------------------------------------------------------------
// Helper functions (private)
// ---------------------------------------------------------------------------

func normalizeMenu(menu string) (string, error) {
	trimmed := strings.TrimSpace(menu)
	if trimmed == "" {
		return "", errors.New("menu is required")
	}
	return "/" + strings.Trim(trimmed, "/"), nil
}

func normalizeItemID(itemID string) (string, error) {
	trimmed := strings.TrimSpace(itemID)
	if trimmed == "" {
		return "", errors.New("item_id is required")
	}
	return trimmed, nil
}

func normalizeCommandPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("command path is required")
	}
	return "/" + strings.Trim(trimmed, "/"), nil
}

func normalizeTag(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", errors.New("tag is required")
	}
	if strings.ContainsAny(tag, " \t\n\r") {
		return "", errors.New("tag must not contain whitespace")
	}
	return tag, nil
}

func normalizeQueries(queries []string) ([]string, error) {
	var normalized []string
	for _, query := range queries {
		trimmed := strings.TrimSpace(query)
		if trimmed == "" {
			return nil, errors.New("query entries must not be empty")
		}
		if strings.ContainsAny(trimmed, " \t\n\r") {
			return nil, fmt.Errorf("query %q must not contain whitespace", query)
		}
		if !strings.HasPrefix(trimmed, "?") {
			trimmed = "?" + trimmed
		}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}

func normalizeAttrs(attrs map[string]any) []struct{ key, value string } {
	var sorted []struct{ key, value string }
	for k, v := range attrs {
		if v != nil {
			sorted = append(sorted, struct{ key, value string }{k, fmt.Sprint(v)})
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].key < sorted[j].key })
	return sorted
}

func buildMenuSentence(menu, action, itemID string, attrs map[string]any) ([]string, error) {
	normalizedMenu, err := normalizeMenu(menu)
	if err != nil {
		return nil, err
	}
	sentence := []string{normalizedMenu + "/" + action}
	if itemID != "" {
		id, err := normalizeItemID(itemID)
		if err != nil {
			return nil, err
		}
		sentence = append(sentence, "=.id="+id)
	}
	for _, attr := range normalizeAttrs(attrs) {
		sentence = append(sentence, "="+attr.key+"="+attr.value)
	}
	return sentence, nil
}

func buildCancelSentence(tag, cancelTag string) ([]string, error) {
	_ = cancelTag // unused in Python version
	normalized, err := normalizeTag(tag)
	if err != nil {
		return nil, err
	}
	return []string{"/cancel", "=tag=" + normalized}, nil
}

func buildCommandSentence(path string, attrs map[string]any, queries []string, tag string) ([]string, error) {
	normalizedPath, err := normalizeCommandPath(path)
	if err != nil {
		return nil, err
	}
	sentence := []string{normalizedPath}
	for _, attr := range normalizeAttrs(attrs) {
		sentence = append(sentence, "="+attr.key+"="+attr.value)
	}
	if len(queries) > 0 {
		normalizedQueries, err := normalizeQueries(queries)
		if err != nil {
			return nil, err
		}
		sentence = append(sentence, normalizedQueries...)
	}
	if tag != "" {
		normalizedTag, err := normalizeTag(tag)
		if err != nil {
			return nil, err
		}
		sentence = append(sentence, ".tag="+normalizedTag)
	}
	return sentence, nil
}

func raiseForErrors(reply *ReplyBundle) error {
	if len(reply.Traps) > 0 {
		trap := reply.Traps[0]
		message := trap["message"]
		if message == "" {
			message = "RouterOS command failed"
		}
		category := trap["category"]
		return &RouterOSError{Message: message, Category: category}
	}
	if reply.Fatal != nil {
		msg := reply.Fatal["message"]
		if msg == "" {
			msg = "RouterOS connection ended unexpectedly"
		}
		return fmt.Errorf("%w: %s", ErrRouterOSFatalError, msg)
	}
	return nil
}

func normalizeMutationResult(reply *ReplyBundle) (map[string]any, error) {
	if err := raiseForErrors(reply); err != nil {
		return nil, err
	}
	if reply.Done != nil && len(reply.Done) > 0 {
		result := make(map[string]any)
		for k, v := range reply.Done {
			result[k] = v
		}
		return result, nil
	}
	if reply.Empty {
		return map[string]any{"success": true, "empty": true}, nil
	}
	return map[string]any{"success": true}, nil
}
