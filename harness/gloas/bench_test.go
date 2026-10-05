// Package gloas benchmarks the Gloas-shaped payload: the same mainnet data
// as the Fulu payload converted to the Gloas types (progressive lists and
// progressive containers), plus the execution payload envelope that holds
// the transactions in Gloas, and the state and block cut to the minimal
// preset (GloasMinState, GloasMinBlock).
package gloas

import (
	"io"
	"path/filepath"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"

	bench "benchkit"
	"realbench/feat"
	plain "realbench/plain/gloas"
	"realbench/types/gloas"
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
	name string
	opts []ssz.DynSszOption
	only map[string]bool // operations measured; nil = all
	// plain: the engine runs on the plain types, the same definitions
	// without generated methods, so that the library has nothing to
	// delegate to and works by reflection alone, in every version of it.
	plain bool
}

var htrOnly = map[string]bool{"HashTreeRoot": true}

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
	reflection := []ssz.DynSszOption{ssz.WithNoFastSsz()}
	es := []engine{{name: "Codegen"}, {name: "Reflection", opts: reflection, plain: true}}
	if async, ok := feat.Async(3); ok {
		es = append(es, engine{name: "CodegenAsync", opts: async, only: htrOnly},
			engine{name: "ReflectionAsync", opts: append(append([]ssz.DynSszOption{}, reflection...), async...), only: htrOnly, plain: true})
	}
	return es
}

// objects are the constructors of the benchmarked objects and the part of
// each whose root is stored with the payload.
type objects struct {
	state, block, envelope        func() any
	stateRoot, blockRoot, envRoot func(any) any
}

func (e engine) objects() objects {
	same := func(v any) any { return v }
	if e.plain {
		return objects{
			state:     func() any { return new(plain.GloasBeaconState) },
			block:     func() any { return new(plain.GloasSignedBeaconBlock) },
			envelope:  func() any { return new(plain.GloasSignedExecutionPayloadEnvelope) },
			stateRoot: same,
			blockRoot: func(v any) any { return v.(*plain.GloasSignedBeaconBlock).Message },
			envRoot:   func(v any) any { return v.(*plain.GloasSignedExecutionPayloadEnvelope).Message },
		}
	}
	return objects{
		state:     func() any { return new(gloas.GloasBeaconState) },
		block:     func() any { return new(gloas.GloasSignedBeaconBlock) },
		envelope:  func() any { return new(gloas.GloasSignedExecutionPayloadEnvelope) },
		stateRoot: same,
		blockRoot: func(v any) any { return v.(*gloas.GloasSignedBeaconBlock).Message },
		envRoot:   func(v any) any { return v.(*gloas.GloasSignedExecutionPayloadEnvelope).Message },
	}
}

func (e engine) codec(ds *ssz.DynSsz, newObj func() any, hashTarget func(any) any) bench.Codec {
	c := dynCodec(ds, newObj, hashTarget)
	c.Only = e.only
	return c
}

func BenchmarkReal(b *testing.B) {
	dir := filepath.Join(bench.DataDir(), "gloas")
	for _, e := range engines() {
		o := e.objects()
		b.Run(e.name, func(b *testing.B) {
			b.Run("GloasState", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, o.state, o.stateRoot), bench.LoadOne(dir, "state"))
			})
			b.Run("GloasBlock", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, o.block, o.blockRoot), bench.LoadOne(dir, "block"))
			})
			b.Run("GloasBlocks", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunSet(b, e.codec(ds, o.block, o.blockRoot), bench.LoadSet(dir, "blocks"))
			})
			b.Run("GloasEnvelope", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, o.envelope, o.envRoot), bench.LoadOne(dir, "envelope"))
			})
			// The minimal preset: the same types with the minimal spec
			// values, on the payload cut to them.
			mdir := filepath.Join(dir, "minimal")
			b.Run("GloasMinState", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(mdir, "spec.json")), e.opts...)
				bench.RunOne(b, restrict(e.codec(ds, o.state, o.stateRoot), minimalOnly), bench.LoadOne(mdir, "state"))
			})
			b.Run("GloasMinBlock", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(mdir, "spec.json")), e.opts...)
				bench.RunOne(b, restrict(e.codec(ds, o.block, o.blockRoot), minimalOnly), bench.LoadOne(mdir, "block"))
			})
		})
	}
}
