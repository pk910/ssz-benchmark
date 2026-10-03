// Package fulu benchmarks methodical-ssz, the generator and runtime Prysm uses, on the Fulu payload of the harness:
// the same bytes the dynamic-ssz engines read, decoded into this library's
// own generated types (types.go is derived from the harness types by
// baselines/gen.sh).
package fulu

import (
	"path/filepath"
	"testing"

	bench "benchkit"
)

const engine = "PrysmSSZ"

type object interface {
	UnmarshalSSZ([]byte) error
	MarshalSSZ() ([]byte, error)
	MarshalSSZTo([]byte) ([]byte, error)
	SizeSSZ() int
}

type hashable interface {
	HashTreeRoot() ([32]byte, error)
}

// only lists the operations this library has for the Fulu objects.
var only = map[string]bool{"Unmarshal": true, "SizeSSZ": true, "Marshal": true, "MarshalTo": true, "HashTreeRoot": true}

func codec(newObj func() object, target func(object) hashable) bench.Codec {
	return bench.Codec{
		Only:      only,
		New:       func() any { return newObj() },
		Unmarshal: func(obj any, data []byte) error { return obj.(object).UnmarshalSSZ(data) },
		Size:      func(obj any) (int, error) { return obj.(object).SizeSSZ(), nil },
		Marshal:   func(obj any) ([]byte, error) { return obj.(object).MarshalSSZ() },
		MarshalTo: func(obj any, buf []byte) ([]byte, error) { return obj.(object).MarshalSSZTo(buf) },
		Root:      func(obj any) ([32]byte, error) { return target(obj.(object)).HashTreeRoot() },
	}
}

// BenchmarkReal/<Engine>/<Object>/<Op>, as in the harness packages.
func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "fulu")
	b.Run(engine, func(b *testing.B) {
		b.Run("FuluState", func(b *testing.B) {
			bench.RunOne(b, codec(func() object { return new(FuluBeaconState) }, func(o object) hashable { return o.(*FuluBeaconState) }), bench.LoadOne(dir, "state"))
		})
		b.Run("FuluBlock", func(b *testing.B) {
			bench.RunOne(b, codec(func() object { return new(ElectraSignedBeaconBlock) }, func(o object) hashable { return o.(*ElectraSignedBeaconBlock).Message }), bench.LoadOne(dir, "block"))
		})
		b.Run("FuluBlocks", func(b *testing.B) {
			bench.RunSet(b, codec(func() object { return new(ElectraSignedBeaconBlock) }, func(o object) hashable { return o.(*ElectraSignedBeaconBlock).Message }), bench.LoadSet(dir, "blocks"))
		})
	})
}
