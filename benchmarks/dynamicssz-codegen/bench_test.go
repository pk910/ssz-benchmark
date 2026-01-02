package dynamicssz

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
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

	// SSZ instances (with codegen support)
	dynSszMainnet *ssz.DynSsz
	dynSszMinimal *ssz.DynSsz
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

	// Minimal preset properties
	minimalPresetBytes, err := os.ReadFile("minimal-preset.yaml")
	if err != nil {
		panic("failed to load minimal-preset.yaml: " + err.Error())
	}
	minimalSpecs := make(map[string]any)
	yaml.Unmarshal(minimalPresetBytes, &minimalSpecs)

	// Create SSZ instances (uses generated code when available)
	dynSszMainnet = ssz.NewDynSsz(nil)
	dynSszMinimal = ssz.NewDynSsz(minimalSpecs)
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

type TestWriter struct {
	data []byte
}

func (w *TestWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *TestWriter) Written() int {
	return len(w.data)
}

func (w *TestWriter) Reset() {
	w.data = w.data[:0]
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

func BenchmarkBlockMainnet_UnmarshalReader(b *testing.B) {
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		reader := bytes.NewReader(blockMainnetData)
		if err := dynSszMainnet.UnmarshalSSZReader(block, reader, len(blockMainnetData)); err != nil {
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

func BenchmarkBlockMainnet_MarshalWriter(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
		b.Fatal(err)
	}
	var writer = &TestWriter{
		data: make([]byte, 0, len(blockMainnetData)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Reset()
		err := dynSszMainnet.MarshalSSZWriter(block, writer)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(writer.data, blockMainnetData) {
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

func BenchmarkBlockMainnet_UnmarshalStreamBuffer(b *testing.B) {
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		dec := sszutils.NewBufferDecoder(blockMainnetData)
		if err := block.UnmarshalSSZDecoder(dynSszMainnet, dec); err != nil {
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

func BenchmarkBlockMainnet_MarshalStreamBuffer(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
		b.Fatal(err)
	}
	bufSize := len(blockMainnetData)
	var enc *sszutils.BufferEncoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc = sszutils.NewBufferEncoder(make([]byte, 0, bufSize))
		if err := block.MarshalSSZEncoder(dynSszMainnet, enc); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(enc.GetBuffer(), blockMainnetData) {
		b.Fatal("marshaled data does not match original")
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

func BenchmarkStateMainnet_UnmarshalReader(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		reader := bytes.NewReader(stateMainnetData)
		if err := dynSszMainnet.UnmarshalSSZReader(state, reader, len(stateMainnetData)); err != nil {
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

func BenchmarkStateMainnet_MarshalWriter(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
		b.Fatal(err)
	}
	var writer = &TestWriter{
		data: make([]byte, 0, len(stateMainnetData)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Reset()
		err := dynSszMainnet.MarshalSSZWriter(state, writer)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(writer.data, stateMainnetData) {
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

func BenchmarkStateMainnet_UnmarshalStreamBuffer(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		dec := sszutils.NewBufferDecoder(stateMainnetData)
		if err := state.UnmarshalSSZDecoder(dynSszMainnet, dec); err != nil {
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

func BenchmarkStateMainnet_MarshalStreamBuffer(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
		b.Fatal(err)
	}
	bufSize := len(stateMainnetData)
	var enc *sszutils.BufferEncoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc = sszutils.NewBufferEncoder(make([]byte, 0, bufSize))
		if err := state.MarshalSSZEncoder(dynSszMainnet, enc); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(enc.GetBuffer(), stateMainnetData) {
		b.Fatal("marshaled data does not match original")
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

func BenchmarkBlockMinimal_UnmarshalReader(b *testing.B) {
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		reader := bytes.NewReader(blockMinimalData)
		if err := dynSszMinimal.UnmarshalSSZReader(block, reader, len(blockMinimalData)); err != nil {
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

func BenchmarkBlockMinimal_MarshalWriter(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
		b.Fatal(err)
	}
	var writer = &TestWriter{
		data: make([]byte, 0, len(blockMinimalData)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Reset()
		err := dynSszMinimal.MarshalSSZWriter(block, writer)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(writer.data, blockMinimalData) {
		for i := 0; i < len(writer.data); i++ {
			if i >= len(blockMinimalData) {
				b.Fatalf("marshaled data is longer than original: %d > %d", len(writer.data), len(blockMinimalData))
			}
			if writer.data[i] != blockMinimalData[i] {
				b.Fatalf("marshaled data does not match original at index %d: got %x, want %x", i, writer.data[i], blockMinimalData[i])
			}
		}
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

func BenchmarkBlockMinimal_UnmarshalStreamBuffer(b *testing.B) {
	var block *SignedBeaconBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block = new(SignedBeaconBlock)
		dec := sszutils.NewBufferDecoder(blockMinimalData)
		if err := block.UnmarshalSSZDecoder(dynSszMinimal, dec); err != nil {
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

func BenchmarkBlockMinimal_MarshalStreamBuffer(b *testing.B) {
	block := new(SignedBeaconBlock)
	if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
		b.Fatal(err)
	}
	bufSize := len(blockMinimalData)
	var enc *sszutils.BufferEncoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc = sszutils.NewBufferEncoder(make([]byte, 0, bufSize))
		if err := block.MarshalSSZEncoder(dynSszMinimal, enc); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(enc.GetBuffer(), blockMinimalData) {
		b.Fatal("marshaled data does not match original")
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

func BenchmarkStateMinimal_UnmarshalReader(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		reader := bytes.NewReader(stateMinimalData)
		if err := dynSszMinimal.UnmarshalSSZReader(state, reader, len(stateMinimalData)); err != nil {
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

func BenchmarkStateMinimal_MarshalWriter(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
		b.Fatal(err)
	}
	var writer = &TestWriter{
		data: make([]byte, 0, len(stateMinimalData)),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Reset()
		err := dynSszMinimal.MarshalSSZWriter(state, writer)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(writer.data, stateMinimalData) {
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

func BenchmarkStateMinimal_UnmarshalStreamBuffer(b *testing.B) {
	var state *BeaconState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state = new(BeaconState)
		dec := sszutils.NewBufferDecoder(stateMinimalData)
		if err := state.UnmarshalSSZDecoder(dynSszMinimal, dec); err != nil {
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

func BenchmarkStateMinimal_MarshalStreamBuffer(b *testing.B) {
	state := new(BeaconState)
	if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
		b.Fatal(err)
	}
	bufSize := len(stateMinimalData)
	var enc *sszutils.BufferEncoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc = sszutils.NewBufferEncoder(make([]byte, 0, bufSize))
		if err := state.MarshalSSZEncoder(dynSszMinimal, enc); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if !bytes.Equal(enc.GetBuffer(), stateMinimalData) {
		b.Fatal("marshaled data does not match original")
	}
}
