// dynssz is a Go adapter that drives dynamic-ssz through benchwrap's
// protocol instead of the kit, on the Fulu objects: what validates the
// wrapper. Its figures must agree with the kit's within the noise floor.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	ssz "github.com/pk910/dynamic-ssz"

	bench "benchkit"
	plain "realbench/plain/fulu"
	"realbench/protocol"
	"realbench/types/fulu"
)

type engine struct {
	name  string
	opts  []ssz.DynSszOption
	state func() any
	block func() any
}

func main() {
	s := protocol.Open()
	s.Thread()
	dir := filepath.Join(bench.DataDir(), "fulu")
	engines := []engine{
		{name: "Codegen", state: func() any { return new(fulu.FuluBeaconState) }, block: func() any { return new(fulu.ElectraSignedBeaconBlock) }},
		{name: "Reflection", opts: []ssz.DynSszOption{ssz.WithNoFastSsz()}, state: func() any { return new(plain.FuluBeaconState) }, block: func() any { return new(plain.ElectraSignedBeaconBlock) }},
	}
	type object struct {
		name   string
		dir    string
		file   string
		newObj func(e engine) func() any
		root   func(any) any
	}
	blockRoot := func(v any) any {
		switch b := v.(type) {
		case *fulu.ElectraSignedBeaconBlock:
			return b.Message
		case *plain.ElectraSignedBeaconBlock:
			return b.Message
		}
		return v
	}
	same := func(v any) any { return v }
	objects := []object{
		{"FuluState", dir, "state", func(e engine) func() any { return e.state }, same},
		{"FuluBlock", dir, "block", func(e engine) func() any { return e.block }, blockRoot},
		{"FuluMinState", filepath.Join(dir, "minimal"), "state", func(e engine) func() any { return e.state }, same},
		{"FuluMinBlock", filepath.Join(dir, "minimal"), "block", func(e engine) func() any { return e.block }, blockRoot},
	}
	ops := []string{"Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"}
	pattern := protocol.Pattern()
	for _, e := range engines {
		for _, o := range objects {
			var leaves []protocol.Leaf
			for _, op := range ops {
				if l := (protocol.Leaf{Engine: e.name, Object: o.name, Op: op}); protocol.Matches(pattern, l) {
					leaves = append(leaves, l)
				}
			}
			if len(leaves) == 0 {
				continue
			}
			ds := ssz.NewDynSsz(bench.LoadSpecs(filepath.Join(o.dir, "spec.json")), e.opts...)
			p := bench.LoadOne(o.dir, o.file)
			newObj := o.newObj(e)
			decoded := newObj()
			if err := ds.UnmarshalSSZ(decoded, p.Data); err != nil {
				for _, l := range leaves {
					s.Fail(l, fmt.Sprintf("decode: %v", err))
				}
				continue
			}
			if out, err := ds.MarshalSSZ(decoded); err != nil || !bytes.Equal(out, p.Data) {
				for _, l := range leaves {
					s.Fail(l, "decoded value does not encode back to the input")
				}
				continue
			}
			if root, err := ds.HashTreeRoot(o.root(decoded)); err != nil || root != p.Root {
				for _, l := range leaves {
					s.Fail(l, fmt.Sprintf("root mismatch: got %x want %x", root, p.Root))
				}
				continue
			}
			buf := make([]byte, 0, len(p.Data))
			for _, l := range leaves {
				switch l.Op {
				case "Unmarshal":
					s.Run(l, func() {
						v := newObj()
						if err := ds.UnmarshalSSZ(v, p.Data); err != nil {
							fail(s, l, err)
						}
					})
				case "SizeSSZ":
					s.Run(l, func() {
						if _, err := ds.SizeSSZ(decoded); err != nil {
							fail(s, l, err)
						}
					})
				case "Marshal":
					s.Run(l, func() {
						if _, err := ds.MarshalSSZ(decoded); err != nil {
							fail(s, l, err)
						}
					})
				case "MarshalTo":
					s.Run(l, func() {
						if _, err := ds.MarshalSSZTo(decoded, buf[:0]); err != nil {
							fail(s, l, err)
						}
					})
				case "HashTreeRoot":
					s.Run(l, func() {
						if _, err := ds.HashTreeRoot(o.root(decoded)); err != nil {
							fail(s, l, err)
						}
					})
				}
			}
		}
	}
}

func fail(s *protocol.Session, l protocol.Leaf, err error) {
	s.Fail(l, err.Error())
	os.Exit(1)
}
