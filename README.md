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

Last updated: 2025-12-11 05:23:45 UTC

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v1) | Unmarshal | 718ns | 1.74KB | 13 |
| fastssz (v1) | Marshal | 346ns | 1.41KB | 1 |
| fastssz (v1) | HashTreeRoot | 12.21µs | 0B | 0 |
| fastssz (v2) | Unmarshal | 1.37µs | 2.18KB | 32 |
| fastssz (v2) | Marshal | 361ns | 1.41KB | 1 |
| fastssz (v2) | HashTreeRoot | 12.22µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 753ns | 1.74KB | 13 |
| dynamic-ssz (codegen) | Marshal | 505ns | 1.41KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 6.39µs | 80B | 3 |
| dynamic-ssz (reflection) | Unmarshal | 3.18µs | 2.24KB | 34 |
| dynamic-ssz (reflection) | Marshal | 1.65µs | 1.41KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.73µs | 80B | 3 |
| karalabe-ssz | Unmarshal | 1.35µs | 1.67KB | 13 |
| karalabe-ssz | Marshal | 837ns | 1.41KB | 1 |
| karalabe-ssz | HashTreeRoot | 11.07µs | 0B | 0 |

### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v1) | Unmarshal | 3.08ms | 4.81MB | 83550 |
| fastssz (v1) | Marshal | 729.17µs | 2.81MB | 1 |
| fastssz (v1) | HashTreeRoot | 11.86ms | 42.59KB | 0 |
| fastssz (v2) | Unmarshal | 3.01ms | 4.81MB | 83563 |
| fastssz (v2) | Marshal | 699.63µs | 2.81MB | 1 |
| fastssz (v2) | HashTreeRoot | 11.90ms | 85.59KB | 0 |
| dynamic-ssz (codegen) | Unmarshal | 701.20µs | 2.81MB | 607 |
| dynamic-ssz (codegen) | Marshal | 676.87µs | 2.81MB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 4.19ms | 7.41KB | 0 |
| dynamic-ssz (reflection) | Unmarshal | 2.30ms | 2.83MB | 1219 |
| dynamic-ssz (reflection) | Marshal | 2.49ms | 2.81MB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.29ms | 64.60KB | 0 |
| karalabe-ssz | Unmarshal | 966.31µs | 2.83MB | 601 |
| karalabe-ssz | Marshal | 812.06µs | 2.81MB | 2 |
| karalabe-ssz | HashTreeRoot | 4.43ms | 35B | 0 |

### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v2) | Unmarshal | 2.20µs | 3.09KB | 51 |
| fastssz (v2) | Marshal | 627ns | 2.05KB | 1 |
| fastssz (v2) | HashTreeRoot | 19.62µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.37µs | 2.60KB | 29 |
| dynamic-ssz (codegen) | Marshal | 759ns | 2.05KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 10.74µs | 320B | 12 |
| dynamic-ssz (reflection) | Unmarshal | 5.57µs | 3.47KB | 65 |
| dynamic-ssz (reflection) | Marshal | 2.62µs | 2.05KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 12.88µs | 320B | 12 |

### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v2) | Unmarshal | 53.82µs | 79.87KB | 698 |
| fastssz (v2) | Marshal | 20.60µs | 73.73KB | 1 |
| fastssz (v2) | HashTreeRoot | 585.88µs | 18B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 28.74µs | 72.39KB | 430 |
| dynamic-ssz (codegen) | Marshal | 18.75µs | 73.73KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 263.43µs | 5B | 0 |
| dynamic-ssz (reflection) | Unmarshal | 139.34µs | 82.75KB | 861 |
| dynamic-ssz (reflection) | Marshal | 81.70µs | 73.73KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 349.92µs | 18B | 0 |

**Note:** karalabe-ssz does not support minimal preset out of the box.

<!-- BENCHMARK_RESULTS_END -->

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
