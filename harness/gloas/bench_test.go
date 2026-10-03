// Package gloas benchmarks the Gloas-shaped payload: the same mainnet data
// as the Fulu payload converted to the Gloas types (progressive lists and
// progressive containers), plus the execution payload envelope that holds
// the transactions in Gloas.
package gloas

import (
	"io"
	"path/filepath"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"

	bench "benchkit"
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
}

var htrOnly = map[string]bool{"HashTreeRoot": true}

func engines() []engine {
	return []engine{
		{name: "Codegen"},
		{name: "Reflection", opts: []ssz.DynSszOption{ssz.WithNoFastSsz(), ssz.WithNoDelegation()}},
		{name: "CodegenAsync", opts: []ssz.DynSszOption{ssz.WithAsyncHashing(3)}, only: htrOnly},
		{name: "ReflectionAsync", opts: []ssz.DynSszOption{ssz.WithNoFastSsz(), ssz.WithNoDelegation(), ssz.WithAsyncHashing(3)}, only: htrOnly},
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
		b.Run(e.name, func(b *testing.B) {
			b.Run("GloasState", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, func() any { return new(gloas.GloasBeaconState) }, func(v any) any { return v }), bench.LoadOne(dir, "state"))
			})
			b.Run("GloasBlock", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, func() any { return new(gloas.GloasSignedBeaconBlock) }, func(v any) any { return v.(*gloas.GloasSignedBeaconBlock).Message }), bench.LoadOne(dir, "block"))
			})
			b.Run("GloasBlocks", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunSet(b, e.codec(ds, func() any { return new(gloas.GloasSignedBeaconBlock) }, func(v any) any { return v.(*gloas.GloasSignedBeaconBlock).Message }), bench.LoadSet(dir, "blocks"))
			})
			b.Run("GloasEnvelope", func(b *testing.B) {
				ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(dir, "spec.json")), e.opts...)
				bench.RunOne(b, e.codec(ds, func() any { return new(gloas.GloasSignedExecutionPayloadEnvelope) }, func(v any) any { return v.(*gloas.GloasSignedExecutionPayloadEnvelope).Message }), bench.LoadOne(dir, "envelope"))
			})
		})
	}
}
