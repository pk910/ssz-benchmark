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

Last updated: 2025-12-02 23:05:59 UTC

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 1.36µs | 2.18KB | 32 |
| fastssz | Marshal | 342ns | 1.41KB | 1 |
| fastssz | HashTreeRoot | 12.25µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 736ns | 1.74KB | 13 |
| dynamic-ssz (codegen) | Marshal | 559ns | 1.41KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 7.76µs | 1.10KB | 22 |
| dynamic-ssz (reflection) | Unmarshal | 42.98µs | 12.99KB | 432 |
| dynamic-ssz (reflection) | Marshal | 70.57µs | 15.90KB | 692 |
| dynamic-ssz (reflection) | HashTreeRoot | 45.79µs | 4.10KB | 274 |
| karalabe-ssz | Unmarshal | 1.36µs | 1.67KB | 13 |
| karalabe-ssz | Marshal | 541ns | 0B | 0 |
| karalabe-ssz | HashTreeRoot | 11.08µs | 0B | 0 |

### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 3.27ms | 4.81MB | 83563 |
| fastssz | Marshal | 705.26µs | 2.81MB | 1 |
| fastssz | HashTreeRoot | 12.01ms | 85.38KB | 0 |
| dynamic-ssz (codegen) | Unmarshal | 724.58µs | 2.81MB | 607 |
| dynamic-ssz (codegen) | Marshal | 513.07µs | 2.81MB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 6.92ms | 2.76MB | 84135 |
| dynamic-ssz (reflection) | Unmarshal | 7.25ms | 3.30MB | 10566 |
| dynamic-ssz (reflection) | Marshal | 5.65ms | 3.30MB | 10076 |
| dynamic-ssz (reflection) | HashTreeRoot | 13.71ms | 168.57KB | 6232 |
| karalabe-ssz | Unmarshal | 848.15µs | 2.83MB | 600 |
| karalabe-ssz | Marshal | 364.38µs | 0B | 0 |
| karalabe-ssz | HashTreeRoot | 4.44ms | 14B | 0 |

### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 2.06µs | 3.09KB | 51 |
| fastssz | Marshal | 552ns | 2.05KB | 1 |
| fastssz | HashTreeRoot | 19.67µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.36µs | 2.60KB | 29 |
| dynamic-ssz (codegen) | Marshal | 822ns | 2.05KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 13.11µs | 1.91KB | 43 |
| dynamic-ssz (reflection) | Unmarshal | 53.48µs | 18.61KB | 544 |
| dynamic-ssz (reflection) | Marshal | 81.85µs | 21.34KB | 800 |
| dynamic-ssz (reflection) | HashTreeRoot | 61.99µs | 4.75KB | 330 |

### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz | Unmarshal | 51.50µs | 79.87KB | 698 |
| fastssz | Marshal | 20.73µs | 73.73KB | 1 |
| fastssz | HashTreeRoot | 587.98µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 28.20µs | 72.39KB | 430 |
| dynamic-ssz (codegen) | Marshal | 15.96µs | 73.73KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 302.04µs | 42.22KB | 1089 |
| dynamic-ssz (reflection) | Unmarshal | 856.71µs | 485.35KB | 8583 |
| dynamic-ssz (reflection) | Marshal | 804.02µs | 488.29KB | 8270 |
| dynamic-ssz (reflection) | HashTreeRoot | 1.31ms | 54.82KB | 4971 |

**Note:** karalabe-ssz does not support minimal preset out of the box.

<!-- BENCHMARK_RESULTS_END -->

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
