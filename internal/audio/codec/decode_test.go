//go:build cgo

package codec

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestDecodeOpusDump(t *testing.T) {
	data, err := os.ReadFile("/tmp/ws_debug_sess_831a4c31811a.opus")
	if err != nil {
		t.Skip(err)
	}
	c, _ := New(FormatOpus, 16000, 1, 60)
	out, _ := os.Create("/tmp/ws_decoded.pcm")
	defer func() { _ = out.Close() }()
	off := 0
	for off+2 <= len(data) {
		l := int(binary.BigEndian.Uint16(data[off:]))
		off += 2
		if off+l > len(data) {
			break
		}
		s, err := c.Decode(data[off : off+l])
		off += l
		if err != nil {
			continue
		}
		b := make([]byte, len(s)*2)
		for i, v := range s {
			b[i*2] = byte(v)
			b[i*2+1] = byte(v >> 8)
		}
		_, _ = out.Write(b)
	}
	t.Logf("decoded -> /tmp/ws_decoded.pcm")
}
