// Package gloas benchmarks the ferranbt/fastssz v2 development branch on the Gloas payload of the harness:
// the same bytes the dynamic-ssz engines read, decoded into this library's
// own generated types (types.go is derived from the harness types by
// baselines/gen.sh).
package gloas

import (
	"path/filepath"
	"testing"

	bench "benchkit"
)

const engine = "FastSSZv2"

type object interface {
	UnmarshalSSZ([]byte) error
	MarshalSSZ() ([]byte, error)
	MarshalSSZTo([]byte) ([]byte, error)
	SizeSSZ() int
}

// only lists the operations this library has for the Gloas objects: it
// has no progressive merkleization, so nothing is hashed.
var only = map[string]bool{"Unmarshal": true, "SizeSSZ": true, "Marshal": true, "MarshalTo": true}

func codec(newObj func() object) bench.Codec {
	return bench.Codec{
		Only:      only,
		New:       func() any { return newObj() },
		Unmarshal: func(obj any, data []byte) error { return obj.(object).UnmarshalSSZ(data) },
		Size:      func(obj any) (int, error) { return obj.(object).SizeSSZ(), nil },
		Marshal:   func(obj any) ([]byte, error) { return obj.(object).MarshalSSZ() },
		MarshalTo: func(obj any, buf []byte) ([]byte, error) { return obj.(object).MarshalSSZTo(buf) },
	}
}

// BenchmarkReal/<Engine>/<Object>/<Op>, as in the harness packages.
func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "gloas")
	b.Run(engine, func(b *testing.B) {
		b.Run("GloasBlock", func(b *testing.B) {
			bench.RunOne(b, codec(func() object { return new(GloasSignedBeaconBlock) }), bench.LoadOne(dir, "block"))
		})
		b.Run("GloasBlocks", func(b *testing.B) {
			bench.RunSet(b, codec(func() object { return new(GloasSignedBeaconBlock) }), bench.LoadSet(dir, "blocks"))
		})
		b.Run("GloasEnvelope", func(b *testing.B) {
			bench.RunOne(b, codec(func() object { return new(GloasSignedExecutionPayloadEnvelope) }), bench.LoadOne(dir, "envelope"))
		})
	})
}
