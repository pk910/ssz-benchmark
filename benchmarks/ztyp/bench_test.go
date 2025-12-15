package ztyp

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/deneb"
	"github.com/protolambda/zrnt/eth2/configs"
	"github.com/protolambda/ztyp/codec"
	"github.com/protolambda/ztyp/tree"
)

type Metadata struct {
	HTR string `json:"htr"`
}

var (
	specMainnet *common.Spec

	blockMainnetData []byte
	stateMainnetData []byte

	blockMainnetHTR common.Root
	stateMainnetHTR common.Root

	// Minimal preset support is currently broken in zrnt - keeping for future use
	// specMinimal     *common.Spec
	// blockMinimalData []byte
	// stateMinimalData []byte
	// blockMinimalHTR common.Root
	// stateMinimalHTR common.Root
)

func init() {
	specMainnet = configs.Mainnet

	var err error
	blockMainnetData, err = os.ReadFile("../../res/block-mainnet.ssz")
	if err != nil {
		panic("failed to load block-mainnet.ssz: " + err.Error())
	}
	stateMainnetData, err = os.ReadFile("../../res/state-mainnet.ssz")
	if err != nil {
		panic("failed to load state-mainnet.ssz: " + err.Error())
	}

	blockMainnetHTR = loadHTR("../../res/block-mainnet-meta.json")
	stateMainnetHTR = loadHTR("../../res/state-mainnet-meta.json")

	// Minimal preset support is currently broken in zrnt - keeping for future use
	// specMinimal = configs.Minimal
	// blockMinimalData, err = os.ReadFile("../../res/block-minimal.ssz")
	// if err != nil {
	// 	panic("failed to load block-minimal.ssz: " + err.Error())
	// }
	// stateMinimalData, err = os.ReadFile("../../res/state-minimal.ssz")
	// if err != nil {
	// 	panic("failed to load state-minimal.ssz: " + err.Error())
	// }
	// blockMinimalHTR = loadHTR("../../res/block-minimal-meta.json")
	// stateMinimalHTR = loadHTR("../../res/state-minimal-meta.json")
}

func loadHTR(path string) common.Root {
	data, err := os.ReadFile(path)
	if err != nil {
		panic("failed to load " + path + ": " + err.Error())
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		panic("failed to parse " + path + ": " + err.Error())
	}
	htrBytes, err := hex.DecodeString(meta.HTR)
	if err != nil {
		panic("failed to decode HTR from " + path + ": " + err.Error())
	}
	var htr common.Root
	copy(htr[:], htrBytes)
	return htr
}

// ========================= BLOCK MAINNET BENCHMARKS =========================

func BenchmarkBlockMainnet_Unmarshal(b *testing.B) {
	var block *deneb.SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(deneb.SignedBeaconBlock)
		err := block.Deserialize(specMainnet, codec.NewDecodingReader(
			bytes.NewReader(blockMainnetData),
			uint64(len(blockMainnetData)),
		))
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr := block.Message.HashTreeRoot(specMainnet, tree.GetHashFn())
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

func BenchmarkBlockMainnet_Marshal(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	err := block.Deserialize(specMainnet, codec.NewDecodingReader(
		bytes.NewReader(blockMainnetData),
		uint64(len(blockMainnetData)),
	))
	if err != nil {
		b.Fatal(err)
	}

	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := block.Serialize(specMainnet, codec.NewEncodingWriter(&buf)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(buf.Bytes(), blockMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkBlockMainnet_HashTreeRoot(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	err := block.Deserialize(specMainnet, codec.NewDecodingReader(
		bytes.NewReader(blockMainnetData),
		uint64(len(blockMainnetData)),
	))
	if err != nil {
		b.Fatal(err)
	}

	var htr common.Root
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		htr = block.Message.HashTreeRoot(specMainnet, tree.GetHashFn())
	}
	b.StopTimer()
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

// ========================= STATE MAINNET BENCHMARKS =========================

func BenchmarkStateMainnet_Unmarshal(b *testing.B) {
	var state *deneb.BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(deneb.BeaconState)
		err := state.Deserialize(specMainnet, codec.NewDecodingReader(
			bytes.NewReader(stateMainnetData),
			uint64(len(stateMainnetData)),
		))
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr := state.HashTreeRoot(specMainnet, tree.GetHashFn())
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}

func BenchmarkStateMainnet_Marshal(b *testing.B) {
	state := new(deneb.BeaconState)
	err := state.Deserialize(specMainnet, codec.NewDecodingReader(
		bytes.NewReader(stateMainnetData),
		uint64(len(stateMainnetData)),
	))
	if err != nil {
		b.Fatal(err)
	}

	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := state.Serialize(specMainnet, codec.NewEncodingWriter(&buf)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(buf.Bytes(), stateMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkStateMainnet_HashTreeRoot(b *testing.B) {
	state := new(deneb.BeaconState)
	err := state.Deserialize(specMainnet, codec.NewDecodingReader(
		bytes.NewReader(stateMainnetData),
		uint64(len(stateMainnetData)),
	))
	if err != nil {
		b.Fatal(err)
	}

	var htr common.Root
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		htr = state.HashTreeRoot(specMainnet, tree.GetHashFn())
	}
	b.StopTimer()
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}

// ========================= BLOCK MINIMAL BENCHMARKS =========================
// Minimal preset support is currently broken in zrnt - keeping for future use

// func BenchmarkBlockMinimal_Unmarshal(b *testing.B) {
// 	var block *deneb.SignedBeaconBlock
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		block = new(deneb.SignedBeaconBlock)
// 		err := block.Deserialize(specMinimal, codec.NewDecodingReader(
// 			bytes.NewReader(blockMinimalData),
// 			uint64(len(blockMinimalData)),
// 		))
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// 	b.StopTimer()
// 	htr := block.Message.HashTreeRoot(specMinimal, tree.GetHashFn())
// 	if htr != blockMinimalHTR {
// 		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMinimalHTR)
// 	}
// }

// func BenchmarkBlockMinimal_Marshal(b *testing.B) {
// 	block := new(deneb.SignedBeaconBlock)
// 	err := block.Deserialize(specMinimal, codec.NewDecodingReader(
// 		bytes.NewReader(blockMinimalData),
// 		uint64(len(blockMinimalData)),
// 	))
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	var buf bytes.Buffer
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		buf.Reset()
// 		if err := block.Serialize(specMinimal, codec.NewEncodingWriter(&buf)); err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// 	b.StopTimer()
// 	if !bytes.Equal(buf.Bytes(), blockMinimalData) {
// 		b.Fatal("marshaled data does not match original")
// 	}
// }

// func BenchmarkBlockMinimal_HashTreeRoot(b *testing.B) {
// 	block := new(deneb.SignedBeaconBlock)
// 	err := block.Deserialize(specMinimal, codec.NewDecodingReader(
// 		bytes.NewReader(blockMinimalData),
// 		uint64(len(blockMinimalData)),
// 	))
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	var htr common.Root
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		htr = block.Message.HashTreeRoot(specMinimal, tree.GetHashFn())
// 	}
// 	b.StopTimer()
// 	if htr != blockMinimalHTR {
// 		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMinimalHTR)
// 	}
// }

// ========================= STATE MINIMAL BENCHMARKS =========================
// Minimal preset support is currently broken in zrnt - keeping for future use

// func BenchmarkStateMinimal_Unmarshal(b *testing.B) {
// 	var state *deneb.BeaconState
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		state = new(deneb.BeaconState)
// 		err := state.Deserialize(specMinimal, codec.NewDecodingReader(
// 			bytes.NewReader(stateMinimalData),
// 			uint64(len(stateMinimalData)),
// 		))
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// 	b.StopTimer()
// 	htr := state.HashTreeRoot(specMinimal, tree.GetHashFn())
// 	if htr != stateMinimalHTR {
// 		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMinimalHTR)
// 	}
// }

// func BenchmarkStateMinimal_Marshal(b *testing.B) {
// 	state := new(deneb.BeaconState)
// 	err := state.Deserialize(specMinimal, codec.NewDecodingReader(
// 		bytes.NewReader(stateMinimalData),
// 		uint64(len(stateMinimalData)),
// 	))
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	var buf bytes.Buffer
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		buf.Reset()
// 		if err := state.Serialize(specMinimal, codec.NewEncodingWriter(&buf)); err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// 	b.StopTimer()
// 	if !bytes.Equal(buf.Bytes(), stateMinimalData) {
// 		b.Fatal("marshaled data does not match original")
// 	}
// }

// func BenchmarkStateMinimal_HashTreeRoot(b *testing.B) {
// 	state := new(deneb.BeaconState)
// 	err := state.Deserialize(specMinimal, codec.NewDecodingReader(
// 		bytes.NewReader(stateMinimalData),
// 		uint64(len(stateMinimalData)),
// 	))
// 	if err != nil {
// 		b.Fatal(err)
// 	}

// 	var htr common.Root
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		htr = state.HashTreeRoot(specMinimal, tree.GetHashFn())
// 	}
// 	b.StopTimer()
// 	if htr != stateMinimalHTR {
// 		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMinimalHTR)
// 	}
// }
