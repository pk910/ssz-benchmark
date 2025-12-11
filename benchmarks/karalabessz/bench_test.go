package karalabessz

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/karalabe/ssz"
)

type Metadata struct {
	HTR string `json:"htr"`
}

var (
	blockMainnetData []byte
	stateMainnetData []byte

	blockMainnetHTR [32]byte
	stateMainnetHTR [32]byte
)

func init() {
	var err error
	blockMainnetData, err = os.ReadFile("../../res/block-mainnet.ssz")
	if err != nil {
		panic("failed to load block-mainnet.ssz: " + err.Error())
	}
	stateMainnetData, err = os.ReadFile("../../res/state-mainnet.ssz")
	if err != nil {
		panic("failed to load state-mainnet.ssz: " + err.Error())
	}

	// Load metadata
	blockMainnetHTR = loadHTR("../../res/block-mainnet-meta.json")
	stateMainnetHTR = loadHTR("../../res/state-mainnet-meta.json")
}

func loadHTR(path string) [32]byte {
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
	var htr [32]byte
	copy(htr[:], htrBytes)
	return htr
}

// ========================= BLOCK MAINNET BENCHMARKS =========================

func BenchmarkBlockMainnet_Unmarshal(b *testing.B) {
	var block *SignedBeaconBlockDeneb
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlockDeneb)
		if err := ssz.DecodeFromBytes(blockMainnetData, block); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr := ssz.HashSequential(block.Message)
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

func BenchmarkBlockMainnet_Marshal(b *testing.B) {
	block := new(SignedBeaconBlockDeneb)
	if err := ssz.DecodeFromBytes(blockMainnetData, block); err != nil {
		b.Fatal(err)
	}
	var buf []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = make([]byte, ssz.SizeOnFork(block, ssz.ForkDeneb))
		if err := ssz.EncodeToBytes(buf, block); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(buf, blockMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkBlockMainnet_HashTreeRoot(b *testing.B) {
	block := new(SignedBeaconBlockDeneb)
	if err := ssz.DecodeFromBytes(blockMainnetData, block); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		htr = ssz.HashSequential(block.Message)
	}
	b.StopTimer()
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

// ========================= STATE MAINNET BENCHMARKS =========================

func BenchmarkStateMainnet_Unmarshal(b *testing.B) {
	var state *BeaconStateDeneb
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconStateDeneb)
		if err := ssz.DecodeFromBytes(stateMainnetData, state); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr := ssz.HashSequential(state)
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}

func BenchmarkStateMainnet_Marshal(b *testing.B) {
	state := new(BeaconStateDeneb)
	if err := ssz.DecodeFromBytes(stateMainnetData, state); err != nil {
		b.Fatal(err)
	}
	var buf []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = make([]byte, ssz.SizeOnFork(state, ssz.ForkDeneb))
		if err := ssz.EncodeToBytes(buf, state); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(buf, stateMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkStateMainnet_HashTreeRoot(b *testing.B) {
	state := new(BeaconStateDeneb)
	if err := ssz.DecodeFromBytes(stateMainnetData, state); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		htr = ssz.HashSequential(state)
	}
	b.StopTimer()
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}
