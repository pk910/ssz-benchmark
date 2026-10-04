// Package fulu benchmarks the Fulu-shaped payload: the extended mainnet
// state, the extended block and the real blocks of the epoch.
package fulu

import (
	"io"
	"path/filepath"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"

	bench "benchkit"
	"realbench/feat"
	"realbench/types/fulu"
)

func dynCodec(ds *ssz.DynSsz, newObj func() any, hashTarget func(any) any) bench.Codec {
	return bench.Codec{
		New:             newObj,
		Unmarshal:       func(obj any, data []byte) error { return ds.UnmarshalSSZ(obj, data) },
		UnmarshalReader: func(obj any, r io.Reader, size int) error { return ds.UnmarshalSSZReader(obj, r, size) },
		Size:            func(obj any) (int, error) { return ds.SizeSSZ(obj) },
		Marshal:         func(obj any) ([]byte, error) { return ds.MarshalSSZ(obj) },
		MarshalTo:       func(obj any, buf []byte) ([]byte, error) { return ds.MarshalSSZTo(obj, buf) },
		MarshalWriter:   func(obj any, w io.Writer) error { return ds.MarshalSSZWriter(obj, w) },
		Root:            func(obj any) ([32]byte, error) { return ds.HashTreeRoot(hashTarget(obj)) },
		Tree: func(obj any) ([]byte, error) {
			t, err := ds.GetTree(hashTarget(obj))
			if err != nil {
				return nil, err
			}
			return t.Hash(), nil
		},
	}
}

type engine struct {
	name  string
	state func(specs map[string]any) bench.Codec
	block func(specs map[string]any) bench.Codec
}

func dynEngine(name string, opts ...ssz.DynSszOption) engine {
	return engine{
		name: name,
		state: func(specs map[string]any) bench.Codec {
			ds := ssz.NewDynSsz(specs, opts...)
			return dynCodec(ds, func() any { return new(fulu.FuluBeaconState) }, func(v any) any { return v })
		},
		block: func(specs map[string]any) bench.Codec {
			ds := ssz.NewDynSsz(specs, opts...)
			return dynCodec(ds, func() any { return new(fulu.ElectraSignedBeaconBlock) }, func(v any) any { return v.(*fulu.ElectraSignedBeaconBlock).Message })
		},
	}
}

// asyncEngine measures the hash tree root with background subtree
// reduction on the cores the benchmark cpuset provides; opts include the
// library's option for it.
func asyncEngine(name string, opts ...ssz.DynSszOption) engine {
	e := dynEngine(name, opts...)
	state, block := e.state, e.block
	only := map[string]bool{"HashTreeRoot": true}
	e.state = func(specs map[string]any) bench.Codec { c := state(specs); c.Only = only; return c }
	e.block = func(specs map[string]any) bench.Codec { c := block(specs); c.Only = only; return c }
	return e
}

// engines lists the engines the library version under test has (package
// feat).
func engines() []engine {
	es := []engine{dynEngine("Codegen")}
	reflection, hasReflection := feat.Reflection()
	async, hasAsync := feat.Async(3)
	if hasReflection {
		es = append(es, dynEngine("Reflection", reflection...))
	}
	if hasAsync {
		es = append(es, asyncEngine("CodegenAsync", async...))
	}
	if hasAsync && hasReflection {
		es = append(es, asyncEngine("ReflectionAsync", append(append([]ssz.DynSszOption{}, reflection...), async...)...))
	}
	return es
}

// BenchmarkReal/<Engine>/<Object>/<Op>. Each object closure loads only its
// own part of the payload, so one leaf run as its own process pages in
// nothing it does not use.
func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "fulu")
	for _, e := range engines() {
		b.Run(e.name, func(b *testing.B) {
			b.Run("FuluState", func(b *testing.B) {
				bench.RunOne(b, e.state(bench.LoadSpecs(filepath.Join(dir, "spec.json"))), bench.LoadOne(dir, "state"))
			})
			b.Run("FuluBlock", func(b *testing.B) {
				bench.RunOne(b, e.block(bench.LoadSpecs(filepath.Join(dir, "spec.json"))), bench.LoadOne(dir, "block"))
			})
			b.Run("FuluBlocks", func(b *testing.B) {
				bench.RunSet(b, e.block(bench.LoadSpecs(filepath.Join(dir, "spec.json"))), bench.LoadSet(dir, "blocks"))
			})
		})
	}
}
