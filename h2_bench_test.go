package fh

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/oarkflow/fh/pkg/hpack"
)

func BenchmarkH2ValidateRequestFields(b *testing.B) {
	fields := []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "example.com"},
		{Name: ":path", Value: "/hello"},
		{Name: "accept", Value: "*/*"},
		{Name: "user-agent", Value: "bench"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := &h2Stream{}
		if err := validateRequestFields(s, fields); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkH2ReadFrameSmallData(b *testing.B) {
	payload := []byte("hello")
	var head [9]byte
	head[2] = byte(len(payload))
	head[3] = h2Data
	binary.BigEndian.PutUint32(head[5:9], 1)
	frame := append(head[:], payload...)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h := newTestH2Conn(&testing.T{})
		h.r = bytes.NewReader(frame)
		if _, err := h.readFrame(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkH2HandleWindowUpdate(b *testing.B) {
	h := newTestH2Conn(&testing.T{})
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], 1)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := h.handleWindowUpdate(h2Frame{typ: h2WindowUpdate, streamID: 0, payload: p[:]}); err != nil {
			b.Fatal(err)
		}
	}
}
