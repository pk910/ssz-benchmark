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

Last updated: 2025-12-11 04:43:30 UTC

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 1.35µs | 2.18KB | 32 |
| fastssz | Marshal | 366ns | 1.41KB | 1 |
| fastssz | HashTreeRoot | 12.28µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 781ns | 1.74KB | 13 |
| dynamic-ssz (codegen) | Marshal | 515ns | 1.41KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 6.41µs | 80B | 3 |
| dynamic-ssz (reflection) | Unmarshal | 3.19µs | 2.24KB | 34 |
| dynamic-ssz (reflection) | Marshal | 1.70µs | 1.41KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.75µs | 80B | 3 |
| karalabe-ssz | Unmarshal | 1.35µs | 1.67KB | 13 |
| karalabe-ssz | Marshal | 540ns | 0B | 0 |
| karalabe-ssz | HashTreeRoot | 11.08µs | 0B | 0 |

### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 3.07ms | 4.81MB | 83563 |
| fastssz | Marshal | 720.29µs | 2.81MB | 1 |
| fastssz | HashTreeRoot | 11.98ms | 85.81KB | 0 |
| dynamic-ssz (codegen) | Unmarshal | 720.68µs | 2.81MB | 607 |
| dynamic-ssz (codegen) | Marshal | 475.30µs | 2.81MB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 4.17ms | 22.17KB | 0 |
| dynamic-ssz (reflection) | Unmarshal | 2.37ms | 2.83MB | 1219 |
| dynamic-ssz (reflection) | Marshal | 2.50ms | 2.81MB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.24ms | 64.59KB | 0 |
| karalabe-ssz | Unmarshal | 906.99µs | 2.83MB | 603 |
| karalabe-ssz | Marshal | 385.62µs | 0B | 0 |
| karalabe-ssz | HashTreeRoot | 4.44ms | 14B | 0 |

### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 2.05µs | 3.09KB | 51 |
| fastssz | Marshal | 587ns | 2.05KB | 1 |
| fastssz | HashTreeRoot | 19.70µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.39µs | 2.60KB | 29 |
| dynamic-ssz (codegen) | Marshal | 751ns | 2.05KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 10.69µs | 320B | 12 |
| dynamic-ssz (reflection) | Unmarshal | 5.64µs | 3.47KB | 65 |
| dynamic-ssz (reflection) | Marshal | 2.67µs | 2.05KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 12.97µs | 320B | 12 |

### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 50.20µs | 79.87KB | 698 |
| fastssz | Marshal | 16.90µs | 73.73KB | 1 |
| fastssz | HashTreeRoot | 587.61µs | 18B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 28.85µs | 72.39KB | 430 |
| dynamic-ssz (codegen) | Marshal | 18.07µs | 73.73KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 264.39µs | 5B | 0 |
| dynamic-ssz (reflection) | Unmarshal | 141.12µs | 82.75KB | 861 |
| dynamic-ssz (reflection) | Marshal | 83.46µs | 73.73KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 350.69µs | 14B | 0 |

**Note:** karalabe-ssz does not support minimal preset out of the box.

<!-- BENCHMARK_RESULTS_END -->

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
