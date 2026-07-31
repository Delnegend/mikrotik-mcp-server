package testutil

import (
	"bytes"
	"io"
	"net"
	"time"
)

type FakeConn struct {
	Buf     bytes.Buffer
	SentBuf bytes.Buffer
	Closed  bool
}

var _ net.Conn = (*FakeConn)(nil)

func NewFakeConn(responses ...[]byte) *FakeConn {
	f := &FakeConn{}
	for _, r := range responses {
		f.Buf.Write(r)
	}
	return f
}

func (f *FakeConn) WriteResponse(data []byte) {
	f.Buf.Write(data)
}

func (f *FakeConn) Read(b []byte) (int, error) {
	n, err := f.Buf.Read(b)
	if err == io.EOF {
		return n, io.EOF
	}
	if n == 0 && f.Buf.Len() == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *FakeConn) Write(b []byte) (int, error) {
	return f.SentBuf.Write(b)
}

func (f *FakeConn) Sent() []byte {
	return f.SentBuf.Bytes()
}

func (f *FakeConn) Dialer() func(string, string, time.Duration) (net.Conn, error) {
	return func(network, addr string, _ time.Duration) (net.Conn, error) {
		return f, nil
	}
}

func (f *FakeConn) Close() error {
	f.Closed = true
	return nil
}

func (f *FakeConn) LocalAddr() net.Addr                { return nil }
func (f *FakeConn) RemoteAddr() net.Addr               { return nil }
func (f *FakeConn) SetDeadline(t time.Time) error      { return nil }
func (f *FakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *FakeConn) SetWriteDeadline(t time.Time) error { return nil }
