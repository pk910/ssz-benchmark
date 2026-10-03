// Package gloas benchmarks karalabe/ssz on the Gloas payload of the
// harness: the same bytes the dynamic-ssz engines read, decoded into this
// library's own generated types (types.go is derived from the harness
// types by baselines/gen.sh).
package gloas

import (
	"io"
	"path/filepath"
	"testing"

	bench "benchkit"

	"github.com/karalabe/ssz"
)

func codec(newObj func() ssz.Object, only map[string]bool) bench.Codec {
	return bench.Codec{
		Only:      only,
		New:       func() any { return newObj() },
		Unmarshal: func(obj any, data []byte) error { return ssz.DecodeFromBytes(data, obj.(ssz.Object)) },
		UnmarshalReader: func(obj any, r io.Reader, size int) error {
			return ssz.DecodeFromStream(r, obj.(ssz.Object), uint32(size))
		},
		Size: func(obj any) (int, error) { return int(ssz.Size(obj.(ssz.Object))), nil },
		Marshal: func(obj any) ([]byte, error) {
			buf := make([]byte, ssz.Size(obj.(ssz.Object)))
			return buf, ssz.EncodeToBytes(buf, obj.(ssz.Object))
		},
		MarshalTo: func(obj any, buf []byte) ([]byte, error) {
			n := int(ssz.Size(obj.(ssz.Object)))
			if cap(buf) < n {
				buf = make([]byte, n)
			}
			buf = buf[:n]
			return buf, ssz.EncodeToBytes(buf, obj.(ssz.Object))
		},
		MarshalWriter: func(obj any, w io.Writer) error { return ssz.EncodeToStream(w, obj.(ssz.Object)) },
	}
}

// serial lists the operations the library has for the Gloas objects: it
// has no progressive merkleization, so nothing is hashed, and its stream
// decoder needs the size up front.
var serial = map[string]bool{"Unmarshal": true, "UnmarshalReader": true, "SizeSSZ": true, "Marshal": true, "MarshalTo": true, "MarshalWriter": true}

// BenchmarkReal/<Engine>/<Object>/<Op>, as in the harness packages.
func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "gloas")
	b.Run("KaralabeSSZ", func(b *testing.B) {
		b.Run("GloasBlock", func(b *testing.B) {
			bench.RunOne(b, codec(func() ssz.Object { return new(GloasSignedBeaconBlock) }, serial), bench.LoadOne(dir, "block"))
		})
		b.Run("GloasBlocks", func(b *testing.B) {
			bench.RunSet(b, codec(func() ssz.Object { return new(GloasSignedBeaconBlock) }, serial), bench.LoadSet(dir, "blocks"))
		})
		b.Run("GloasEnvelope", func(b *testing.B) {
			bench.RunOne(b, codec(func() ssz.Object { return new(GloasSignedExecutionPayloadEnvelope) }, serial), bench.LoadOne(dir, "envelope"))
		})
	})
}
