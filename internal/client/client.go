package client

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrRouterOSFatalError     = errors.New("routeros: fatal error")
	ErrRouterOSAuthError      = errors.New("routeros: auth error")
	ErrRouterOSTransportError = errors.New("routeros: transport error")
)

var NetDial = func(network, addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

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
	Tag          string
	Records      []map[string]string
	Done         map[string]string
	Traps        []map[string]string
	Fatal        map[string]string
	Empty        bool
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
	rawConn, err := NetDial("tcp", addr, c.timeout)
	if err != nil {
		return fmt.Errorf("%w: failed to connect to %s: %v", ErrRouterOSTransportError, addr, err)
	}

	if c.useSSL {
		tlsConfig := &tls.Config{
			ServerName:         c.host,
			InsecureSkipVerify: !c.tlsVerify,
		}
		if c.tlsVerify && len(c.tlsCAFiles) > 0 {
			certPool := x509.NewCertPool()
			for _, f := range c.tlsCAFiles {
				pem, err := os.ReadFile(f)
				if err != nil {
					rawConn.Close()
					return fmt.Errorf("%w: failed to read CA cert %s: %v", ErrRouterOSTransportError, f, err)
				}
				if !certPool.AppendCertsFromPEM(pem) {
					rawConn.Close()
					return fmt.Errorf("%w: no valid CA cert found in %s", ErrRouterOSTransportError, f)
				}
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
	return nil
}

func (c *RouterOSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
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
	reply, err := c.command("/login", map[string]any{"name": c.username, "password": c.password})
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
	normalizedMenu := normalizeMenu(menu)
	words := []string{normalizedMenu + "/print"}
	if len(proplist) > 0 {
		words = append(words, "=.proplist="+strings.Join(proplist, ","))
	}
	for _, attr := range normalizeAttrs(attrs) {
		words = append(words, "="+attr.key+"="+attr.value)
	}
	for _, query := range normalizeQueries(queries) {
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
	reply, err := c.execute(buildMenuSentence(menu, "add", "", attrs))
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Set(menu, itemID string, attrs map[string]any) (map[string]any, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, errors.New("item_id is required")
	}
	reply, err := c.execute(buildMenuSentence(menu, "set", itemID, attrs))
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Remove(menu, itemID string) (map[string]any, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, errors.New("item_id is required")
	}
	reply, err := c.execute(buildMenuSentence(menu, "remove", itemID, nil))
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) Run(path string, attrs map[string]any, queries []string, tag string) (any, error) {
	words := buildCommandSentence(path, attrs, queries, tag)
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

func (c *RouterOSClient) Listen(menu string, proplist, queries []string, attrs map[string]any, tag string, maxEvents int) (*ListenResult, error) {
	c.execMu.Lock()
	defer c.execMu.Unlock()

	if maxEvents < 1 {
		return nil, errors.New("max_events must be at least 1")
	}

	listenTag := tag
	if listenTag == "" {
		listenTag = c.generateTag("listen")
	}
	cancelTag := listenTag + "-cancel"

	normalizedMenu := normalizeMenu(menu)
	words := []string{normalizedMenu + "/listen"}
	if len(proplist) > 0 {
		words = append(words, "=.proplist="+strings.Join(proplist, ","))
	}
	for _, attr := range normalizeAttrs(attrs) {
		words = append(words, "="+attr.key+"="+attr.value)
	}
	for _, query := range normalizeQueries(queries) {
		words = append(words, query)
	}
	words = append(words, ".tag="+listenTag)

	result := &ListenResult{Tag: listenTag}

	if err := c.writeSentence(words); err != nil {
		return nil, err
	}

	cancelSent := false
	listenDone := false
	cancelDone := false

	for {
		c.setDeadline()
		sentence, err := readSentence(c.conn)
		if err != nil {
			msg := err.Error()
			// On timeout, send cancel
			if !cancelSent && (errors.Is(err, ErrRouterOSTransportError) || strings.Contains(strings.ToLower(msg), "timeout")) {
				result.Cancelled = true
				cancelWords := buildCancelSentence(listenTag, "")
				if writeErr := c.writeSentence(cancelWords); writeErr != nil {
					return nil, writeErr
				}
				cancelSent = true
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
					cancelWords := buildCancelSentence(listenTag, "")
					if writeErr := c.writeSentence(cancelWords); writeErr != nil {
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
	reply, err := c.execute(buildCancelSentence(tag, ""))
	if err != nil {
		return nil, err
	}
	return normalizeMutationResult(reply)
}

func (c *RouterOSClient) command(path string, attrs map[string]any) (*ReplyBundle, error) {
	words := buildCommandSentence(path, attrs, nil, "")
	reply, err := c.execute(words)
	if err != nil {
		return nil, err
	}
	if reply.Fatal != nil {
		msg := reply.Fatal["message"]
		if msg == "" {
			msg = "RouterOS connection ended unexpectedly"
		}
		return nil, fmt.Errorf("%w: %s", ErrRouterOSFatalError, msg)
	}
	return reply, nil
}

func (c *RouterOSClient) execute(words []string) (*ReplyBundle, error) {
	c.execMu.Lock()
	defer c.execMu.Unlock()

	if c.conn == nil {
		if err := c.Open(); err != nil {
			return nil, err
		}
	}
	c.setDeadline()
	if err := c.writeSentence(words); err != nil {
		return nil, err
	}
	var sentences [][]string
	for {
		sentence, err := readSentence(c.conn)
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

func (c *RouterOSClient) SetConn(conn net.Conn) { c.conn = conn }

func (c *RouterOSClient) generateTag(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)[:12]
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

func normalizeMenu(menu string) string {
	trimmed := strings.TrimSpace(menu)
	if trimmed == "" {
		panic("menu is required")
	}
	return "/" + strings.Trim(trimmed, "/")
}

func normalizeItemID(itemID string) string {
	trimmed := strings.TrimSpace(itemID)
	if trimmed == "" {
		panic("item_id is required")
	}
	return trimmed
}

func normalizeCommandPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		panic("command path is required")
	}
	return "/" + strings.Trim(trimmed, "/")
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		panic("tag is required")
	}
	if strings.ContainsAny(tag, " \t\n\r") {
		panic("tag must not contain whitespace")
	}
	return tag
}

func normalizeQueries(queries []string) []string {
	var normalized []string
	for _, query := range queries {
		trimmed := strings.TrimSpace(query)
		if trimmed == "" {
			panic("query entries must not be empty")
		}
		if strings.ContainsAny(trimmed, " \t\n\r") {
			panic("query '" + query + "' must not contain whitespace")
		}
		if !strings.HasPrefix(trimmed, "?") {
			trimmed = "?" + trimmed
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func normalizeAttrs(attrs map[string]any) []struct{ key, value string } {
	type kv struct{ k, v string }
	var sorted []kv
	for k, v := range attrs {
		if v != nil {
			sorted = append(sorted, kv{k, stringifyValue(v)})
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].k < sorted[j].k })
	result := make([]struct{ key, value string }, len(sorted))
	for i, item := range sorted {
		result[i] = struct{ key, value string }{item.k, item.v}
	}
	return result
}

func stringifyValue(value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func buildMenuSentence(menu, action, itemID string, attrs map[string]any) []string {
	sentence := []string{normalizeMenu(menu) + "/" + action}
	if itemID != "" {
		sentence = append(sentence, "=.id="+normalizeItemID(itemID))
	}
	for _, attr := range normalizeAttrs(attrs) {
		sentence = append(sentence, "="+attr.key+"="+attr.value)
	}
	return sentence
}

func buildCancelSentence(tag, cancelTag string) []string {
	_ = cancelTag // unused in Python version
	return []string{"/cancel", "=tag=" + normalizeTag(tag)}
}

func buildCommandSentence(path string, attrs map[string]any, queries []string, tag string) []string {
	sentence := []string{normalizeCommandPath(path)}
	for _, attr := range normalizeAttrs(attrs) {
		sentence = append(sentence, "="+attr.key+"="+attr.value)
	}
	for _, query := range normalizeQueries(queries) {
		sentence = append(sentence, query)
	}
	if tag != "" {
		sentence = append(sentence, ".tag="+normalizeTag(tag))
	}
	return sentence
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
