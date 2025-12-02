package dynamicsszreflection

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"gopkg.in/yaml.v2"
)

type Metadata struct {
	HTR string `json:"htr"`
}

var (
	blockMainnetData []byte
	stateMainnetData []byte
	blockMinimalData []byte
	stateMinimalData []byte

	blockMainnetHTR [32]byte
	stateMainnetHTR [32]byte
	blockMinimalHTR [32]byte
	stateMinimalHTR [32]byte

	// Dynamic SSZ instance for mainnet (pure reflection, no fastssz)
	dynSszMainnet *dynssz.DynSsz

	// Dynamic SSZ instance for minimal preset (pure reflection, no fastssz)
	dynSszMinimal *dynssz.DynSsz
)

func init() {
	var err error

	// Load test data
	blockMainnetData, err = os.ReadFile("../../res/block-mainnet.ssz")
	if err != nil {
		panic("failed to load block-mainnet.ssz: " + err.Error())
	}
	stateMainnetData, err = os.ReadFile("../../res/state-mainnet.ssz")
	if err != nil {
		panic("failed to load state-mainnet.ssz: " + err.Error())
	}
	blockMinimalData, err = os.ReadFile("../../res/block-minimal.ssz")
	if err != nil {
		panic("failed to load block-minimal.ssz: " + err.Error())
	}
	stateMinimalData, err = os.ReadFile("../../res/state-minimal.ssz")
	if err != nil {
		panic("failed to load state-minimal.ssz: " + err.Error())
	}

	// Load metadata
	blockMainnetHTR = loadHTR("../../res/block-mainnet-meta.json")
	stateMainnetHTR = loadHTR("../../res/state-mainnet-meta.json")
	blockMinimalHTR = loadHTR("../../res/block-minimal-meta.json")
	stateMinimalHTR = loadHTR("../../res/state-minimal-meta.json")

	// Load minimal preset
	minimalPresetBytes, err := os.ReadFile("minimal-preset.yaml")
	if err != nil {
		panic("failed to load minimal-preset.yaml: " + err.Error())
	}
	minimalSpecs := make(map[string]any)
	yaml.Unmarshal(minimalPresetBytes, &minimalSpecs)

	// Initialize dynamic SSZ with pure reflection mode (NoFastSsz=true)
	// This disables any generated SSZ code and uses only reflection
	dynSszMainnet = dynssz.NewDynSsz(nil)
	dynSszMainnet.NoFastSsz = true // Force pure reflection mode

	dynSszMinimal = dynssz.NewDynSsz(minimalSpecs)
	dynSszMinimal.NoFastSsz = true // Force pure reflection mode
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
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr, err := dynSszMainnet.HashTreeRoot(block.Message)
	if err != nil {
		b.Fatal(err)
	}
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

func BenchmarkBlockMainnet_Marshal(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
		b.Fatal(err)
	}
	var data []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		data, err = dynSszMainnet.MarshalSSZ(block)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(data, blockMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkBlockMainnet_HashTreeRoot(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		htr, err = dynSszMainnet.HashTreeRoot(block.Message)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if htr != blockMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMainnetHTR)
	}
}

// ========================= STATE MAINNET BENCHMARKS =========================

func BenchmarkStateMainnet_Unmarshal(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr, err := dynSszMainnet.HashTreeRoot(state)
	if err != nil {
		b.Fatal(err)
	}
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}

func BenchmarkStateMainnet_Marshal(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
		b.Fatal(err)
	}
	var data []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		data, err = dynSszMainnet.MarshalSSZ(state)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(data, stateMainnetData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkStateMainnet_HashTreeRoot(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		htr, err = dynSszMainnet.HashTreeRoot(state)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if htr != stateMainnetHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMainnetHTR)
	}
}

// ========================= BLOCK MINIMAL BENCHMARKS =========================

func BenchmarkBlockMinimal_Unmarshal(b *testing.B) {
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr, err := dynSszMinimal.HashTreeRoot(block.Message)
	if err != nil {
		b.Fatal(err)
	}
	if htr != blockMinimalHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMinimalHTR)
	}
}

func BenchmarkBlockMinimal_Marshal(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
		b.Fatal(err)
	}
	var data []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		data, err = dynSszMinimal.MarshalSSZ(block)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(data, blockMinimalData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkBlockMinimal_HashTreeRoot(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		htr, err = dynSszMinimal.HashTreeRoot(block.Message)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if htr != blockMinimalHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, blockMinimalHTR)
	}
}

// ========================= STATE MINIMAL BENCHMARKS =========================

func BenchmarkStateMinimal_Unmarshal(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	htr, err := dynSszMinimal.HashTreeRoot(state)
	if err != nil {
		b.Fatal(err)
	}
	if htr != stateMinimalHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMinimalHTR)
	}
}

func BenchmarkStateMinimal_Marshal(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
		b.Fatal(err)
	}
	var data []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		data, err = dynSszMinimal.MarshalSSZ(state)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(data, stateMinimalData) {
		b.Fatal("marshaled data does not match original")
	}
}

func BenchmarkStateMinimal_HashTreeRoot(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
		b.Fatal(err)
	}
	var htr [32]byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		htr, err = dynSszMinimal.HashTreeRoot(state)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if htr != stateMinimalHTR {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, stateMinimalHTR)
	}
}
