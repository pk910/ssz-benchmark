// Package fulu benchmarks the Fulu-shaped payload: the extended mainnet
// state, the extended block and the real blocks of the epoch, and the
// state and block cut to the minimal preset (FuluMinState, FuluMinBlock),
// where every spec value the types depend on differs from its default.
package fulu

import (
	"io"
	"path/filepath"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"

	bench "benchkit"
	"realbench/feat"
	plain "realbench/plain/fulu"
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

// reflectionEngine runs on the plain types: the same definitions without
// generated methods, so that the library has nothing to delegate to and
// works by reflection alone, in every version of it.
func reflectionEngine(name string, opts ...ssz.DynSszOption) engine {
	opts = append([]ssz.DynSszOption{ssz.WithNoFastSsz()}, opts...)
	return engine{
		name: name,
		state: func(specs map[string]any) bench.Codec {
			ds := ssz.NewDynSsz(specs, opts...)
			return dynCodec(ds, func() any { return new(plain.FuluBeaconState) }, func(v any) any { return v })
		},
		block: func(specs map[string]any) bench.Codec {
			ds := ssz.NewDynSsz(specs, opts...)
			return dynCodec(ds, func() any { return new(plain.ElectraSignedBeaconBlock) }, func(v any) any { return v.(*plain.ElectraSignedBeaconBlock).Message })
		},
	}
}

// hashOnly restricts an engine to the hash tree root: the async engines
// measure it with background subtree reduction on the cores the benchmark
// cpuset provides.
func hashOnly(e engine) engine {
	state, block := e.state, e.block
	only := map[string]bool{"HashTreeRoot": true}
	e.state = func(specs map[string]any) bench.Codec { c := state(specs); c.Only = only; return c }
	e.block = func(specs map[string]any) bench.Codec { c := block(specs); c.Only = only; return c }
	return e
}

// minimalOnly are the operations measured on the minimal objects: the
// three that carry every spec-dependent size and limit; the other six are
// variants of them.
var minimalOnly = map[string]bool{"Unmarshal": true, "Marshal": true, "HashTreeRoot": true}

// restrict keeps the operations of only that the codec has.
func restrict(c bench.Codec, only map[string]bool) bench.Codec {
	keep := make(map[string]bool, len(only))
	for op := range only {
		if c.Only == nil || c.Only[op] {
			keep[op] = true
		}
	}
	c.Only = keep
	return c
}

// engines lists the engines the library version under test has (package
// feat).
func engines() []engine {
	es := []engine{dynEngine("Codegen"), reflectionEngine("Reflection")}
	if async, ok := feat.Async(3); ok {
		es = append(es, hashOnly(dynEngine("CodegenAsync", async...)), hashOnly(reflectionEngine("ReflectionAsync", async...)))
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
			// The minimal preset: the same types with the minimal spec
			// values, on the payload cut to them.
			mdir := filepath.Join(dir, "minimal")
			b.Run("FuluMinState", func(b *testing.B) {
				bench.RunOne(b, restrict(e.state(bench.LoadSpecs(filepath.Join(mdir, "spec.json"))), minimalOnly), bench.LoadOne(mdir, "state"))
			})
			b.Run("FuluMinBlock", func(b *testing.B) {
				bench.RunOne(b, restrict(e.block(bench.LoadSpecs(filepath.Join(mdir, "spec.json"))), minimalOnly), bench.LoadOne(mdir, "block"))
			})
		})
	}
}
