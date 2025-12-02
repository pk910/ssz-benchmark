# SSZ Benchmark Suite

A comprehensive benchmarking suite for comparing SSZ (Simple Serialize) library implementations in Go.

## Libraries Tested

- **[fastssz](https://github.com/ferranbt/fastssz)** - Code generation based SSZ library
- **[dynamic-ssz](https://github.com/pk910/dynamic-ssz)** - Dynamic SSZ library with support for both reflection and code generation modes
- **[karalabe-ssz](https://github.com/karalabe/ssz)** - High-performance SSZ library

## Test Data

The benchmarks use real Ethereum consensus layer data:
- **Block Mainnet**: Deneb signed beacon block from mainnet
- **State Mainnet**: Deneb beacon state from mainnet
- **Block Minimal**: Deneb signed beacon block with minimal preset
- **State Minimal**: Deneb beacon state with minimal preset

## Benchmarks

Each library is tested for the following operations:
- **Unmarshal**: Deserialize SSZ bytes into Go structures
- **Marshal**: Serialize Go structures into SSZ bytes
- **HashTreeRoot**: Compute the Merkle root of the structure

### dynamic-ssz Modes

The dynamic-ssz library supports two modes:
- **Codegen** (`benchmarks/dynamicssz`): Uses pre-generated SSZ code (similar to fastssz). The generated code provides fastssz-compatible methods that dynamic-ssz can use for better performance.
- **Reflection** (`benchmarks/dynamicssz-reflection`): Uses pure Go reflection for SSZ encoding/decoding without any generated code. This mode is fully dynamic and can handle different spec presets at runtime.

## Running Benchmarks Locally

```bash
# Run fastssz benchmarks
cd benchmarks/fastssz
go test -run=^$ -bench=. -benchmem

# Run dynamic-ssz benchmarks (with generated code)
cd benchmarks/dynamicssz
go test -run=^$ -bench=. -benchmem

# Run dynamic-ssz benchmarks (pure reflection, no generated code)
cd benchmarks/dynamicssz-reflection
go test -run=^$ -bench=. -benchmem

# Run karalabe-ssz benchmarks
cd benchmarks/karalabessz
go test -run=^$ -bench=. -benchmem
```

## Project Structure

```
ssz-benchmark/
├── benchmarks/
│   ├── fastssz/              # fastssz benchmark module
│   ├── dynamicssz/           # dynamic-ssz with generated code
│   ├── dynamicssz-reflection/# dynamic-ssz pure reflection (no codegen)
│   └── karalabessz/          # karalabe-ssz benchmark module
├── res/                      # Test data files
│   ├── block-mainnet.ssz
│   ├── state-mainnet.ssz
│   ├── block-minimal.ssz
│   └── state-minimal.ssz
└── .github/workflows/        # CI/CD workflows
```

<!-- BENCHMARK_RESULTS_START -->
## Benchmark Results

Last updated: 2025-12-02 23:43:40 UTC

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 1.90µs | 2.18KB | 32 |
| fastssz | Marshal | 449ns | 1.41KB | 1 |
| fastssz | HashTreeRoot | 6.71µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.33µs | 1.74KB | 13 |
| dynamic-ssz (codegen) | Marshal | 1.11µs | 1.41KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 4.88µs | 1.10KB | 22 |
| dynamic-ssz (reflection) | Unmarshal | 31.74µs | 11.17KB | 231 |
| dynamic-ssz (reflection) | Marshal | 42.66µs | 12.84KB | 336 |
| dynamic-ssz (reflection) | HashTreeRoot | 22.98µs | 3.15KB | 155 |
| karalabe-ssz | Unmarshal | 1.27µs | 1.68KB | 13 |
| karalabe-ssz | Marshal | 296ns | 0B | 0 |
| karalabe-ssz | HashTreeRoot | 6.71µs | 0B | 0 |

### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 3.75ms | 4.81MB | 83563 |
| fastssz | Marshal | 845.92µs | 2.81MB | 1 |
| fastssz | HashTreeRoot | 6.85ms | 48.68KB | 0 |
| dynamic-ssz (codegen) | Unmarshal | 874.66µs | 2.81MB | 607 |
| dynamic-ssz (codegen) | Marshal | 984.72µs | 2.81MB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 5.18ms | 2.73MB | 84135 |
| dynamic-ssz (reflection) | Unmarshal | 5.28ms | 3.27MB | 6801 |
| dynamic-ssz (reflection) | Marshal | 3.74ms | 3.27MB | 6249 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.72ms | 81.89KB | 2477 |
| karalabe-ssz | Unmarshal | 1.11ms | 2.83MB | 603 |
| karalabe-ssz | Marshal | 191.96µs | 1B | 0 |
| karalabe-ssz | HashTreeRoot | 3.66ms | 24B | 0 |

### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 3.31µs | 3.09KB | 51 |
| fastssz | Marshal | 1.07µs | 2.05KB | 1 |
| fastssz | HashTreeRoot | 10.62µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.61µs | 2.60KB | 29 |
| dynamic-ssz (codegen) | Marshal | 1.61µs | 2.05KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 8.03µs | 1.91KB | 43 |
| dynamic-ssz (reflection) | Unmarshal | 44.52µs | 16.33KB | 298 |
| dynamic-ssz (reflection) | Marshal | 55.25µs | 17.80KB | 390 |
| dynamic-ssz (reflection) | HashTreeRoot | 30.97µs | 3.52KB | 175 |

### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 65.61µs | 79.87KB | 698 |
| fastssz | Marshal | 34.17µs | 73.73KB | 1 |
| fastssz | HashTreeRoot | 321.88µs | 8B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 54.06µs | 72.39KB | 430 |
| dynamic-ssz (codegen) | Marshal | 27.64µs | 73.73KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 213.39µs | 42.28KB | 1089 |
| dynamic-ssz (reflection) | Unmarshal | 960.81µs | 459.49KB | 5363 |
| dynamic-ssz (reflection) | Marshal | 663.58µs | 461.94KB | 4988 |
| dynamic-ssz (reflection) | HashTreeRoot | 670.86µs | 29.17KB | 1761 |

**Note:** karalabe-ssz does not support minimal preset out of the box.

<!-- BENCHMARK_RESULTS_END -->

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
