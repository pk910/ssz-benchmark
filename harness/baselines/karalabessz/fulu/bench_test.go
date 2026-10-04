// Package fulu benchmarks karalabe/ssz on the Fulu payload of the
// harness: the same bytes the dynamic-ssz engines read, decoded into this
// library's own generated types (types.go is derived from the harness
// types by baselines/gen.sh).
package fulu

import (
	"io"
	"path/filepath"
	"testing"

	bench "benchkit"

	"github.com/karalabe/ssz"
)

func codec(newObj func() ssz.Object, only map[string]bool, hash func(ssz.Object) [32]byte, target func(ssz.Object) ssz.Object) bench.Codec {
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
		Root:          func(obj any) ([32]byte, error) { return hash(target(obj.(ssz.Object))), nil },
	}
}

// The stream decoder needs the size up front, so there is no decoding of
// an unknown-size stream.
var (
	all     = map[string]bool{"Unmarshal": true, "UnmarshalReader": true, "SizeSSZ": true, "Marshal": true, "MarshalTo": true, "MarshalWriter": true, "HashTreeRoot": true}
	htrOnly = map[string]bool{"HashTreeRoot": true}
)

type engine struct {
	name string
	only map[string]bool
	hash func(ssz.Object) [32]byte
}

// KaralabeSSZAsync is the library's concurrent hasher, next to the async
// engines of dynamic-ssz.
var engines = []engine{
	{"KaralabeSSZ", all, ssz.HashSequential},
	{"KaralabeSSZAsync", htrOnly, ssz.HashConcurrent},
}

// BenchmarkReal/<Engine>/<Object>/<Op>, as in the harness packages.
func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "fulu")
	for _, e := range engines {
		b.Run(e.name, func(b *testing.B) {
			b.Run("FuluBlock", func(b *testing.B) {
				bench.RunOne(b, codec(func() ssz.Object { return new(ElectraSignedBeaconBlock) }, e.only, e.hash, func(o ssz.Object) ssz.Object { return o.(*ElectraSignedBeaconBlock).Message }), bench.LoadOne(dir, "block"))
			})
			b.Run("FuluBlocks", func(b *testing.B) {
				bench.RunSet(b, codec(func() ssz.Object { return new(ElectraSignedBeaconBlock) }, e.only, e.hash, func(o ssz.Object) ssz.Object { return o.(*ElectraSignedBeaconBlock).Message }), bench.LoadSet(dir, "blocks"))
			})
		})
	}
}
