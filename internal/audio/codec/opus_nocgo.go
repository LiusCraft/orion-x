//go:build !cgo

package codec

import "fmt"

func newOpusCodec(_, _, _ int) (Codec, error) {
	return nil, fmt.Errorf("codec: Opus requires CGO_ENABLED=1 and libopus")
}
