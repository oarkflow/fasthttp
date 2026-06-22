package websocket

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/oarkflow/fh"
)

func TestUpgradeAndEcho(t *testing.T) {
	app := fh.New()
	app.Get("/ws", New(func(ws *Conn) error {
		opcode, message, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		return ws.WriteMessage(opcode, message)
	}))
	client := runPipeApp(t, app)
	frame := maskedFrame(Text, []byte("hello"))
	head := "GET /ws HTTP/1.1\r\nHost: local\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	go func() { _, _ = client.Write(append([]byte(head), frame...)) }()
	reader := bufio.NewReader(client)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	var header [2]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x80|Text || header[1] != 5 {
		t.Fatalf("bad frame header: %x", header)
	}
	payload := make([]byte, 5)
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("got %q", payload)
	}
}

type singleListener struct {
	conn net.Conn
	done chan struct{}
	once sync.Once
}

func newSingleListener(conn net.Conn) *singleListener {
	return &singleListener{conn: conn, done: make(chan struct{})}
}
func (l *singleListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		return conn, nil
	}
	<-l.done
	return nil, net.ErrClosed
}
func (l *singleListener) Close() error { l.once.Do(func() { close(l.done) }); return nil }
func (*singleListener) Addr() net.Addr { return testAddr("pipe") }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

func runPipeApp(t *testing.T, app *fh.App) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	listener := newSingleListener(server)
	go func() { _ = app.Serve(listener) }()
	t.Cleanup(func() { _ = client.Close(); _ = app.ShutdownWithTimeout(time.Second) })
	return client
}

func maskedFrame(opcode byte, payload []byte) []byte {
	mask := [4]byte{1, 2, 3, 4}
	frame := []byte{0x80 | opcode}
	if len(payload) < 126 {
		frame = append(frame, 0x80|byte(len(payload)))
	} else {
		frame = append(frame, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(frame[len(frame)-2:], uint16(len(payload)))
	}
	frame = append(frame, mask[:]...)
	for i, value := range payload {
		frame = append(frame, value^mask[i&3])
	}
	return frame
}
