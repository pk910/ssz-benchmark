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

Last updated: 2025-12-12 02:49:35 UTC

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v1) | Unmarshal | 1.59µs | 1.74KB | 13 |
| fastssz (v1) | Marshal | 779ns | 1.41KB | 1 |
| fastssz (v1) | HashTreeRoot | 6.46µs | 0B | 0 |
| fastssz (v2) | Unmarshal | 2.71µs | 2.18KB | 32 |
| fastssz (v2) | Marshal | 830ns | 1.41KB | 1 |
| fastssz (v2) | HashTreeRoot | 6.48µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.59µs | 1.74KB | 13 |
| dynamic-ssz (codegen) | Marshal | 1.33µs | 1.41KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 3.99µs | 80B | 3 |
| dynamic-ssz (reflection) | Unmarshal | 4.22µs | 2.25KB | 34 |
| dynamic-ssz (reflection) | Marshal | 2.00µs | 1.41KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 4.55µs | 80B | 3 |
| dynamic-ssz (reflection) | UnmarshalReader | 6.93µs | 4.55KB | 72 |
| dynamic-ssz (reflection) | MarshalWriter | 2.87µs | 1.20KB | 29 |
| karalabe-ssz | Unmarshal | 2.29µs | 1.68KB | 13 |
| karalabe-ssz | Marshal | 1.52µs | 1.41KB | 1 |
| karalabe-ssz | HashTreeRoot | 6.66µs | 0B | 0 |
| karalabe-ssz | UnmarshalReader | 2.86µs | 1.72KB | 14 |
| karalabe-ssz | MarshalWriter | 347ns | 0B | 0 |

### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v1) | Unmarshal | 4.42ms | 4.81MB | 83550 |
| fastssz (v1) | Marshal | 1.31ms | 2.81MB | 1 |
| fastssz (v1) | HashTreeRoot | 6.61ms | 59.30KB | 0 |
| fastssz (v2) | Unmarshal | 4.66ms | 4.81MB | 83563 |
| fastssz (v2) | Marshal | 1.25ms | 2.81MB | 1 |
| fastssz (v2) | HashTreeRoot | 6.62ms | 23.98KB | 0 |
| dynamic-ssz (codegen) | Unmarshal | 1.52ms | 2.81MB | 607 |
| dynamic-ssz (codegen) | Marshal | 1.11ms | 2.81MB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 3.41ms | 18.23KB | 0 |
| dynamic-ssz (reflection) | Unmarshal | 3.10ms | 2.83MB | 1219 |
| dynamic-ssz (reflection) | Marshal | 2.33ms | 2.81MB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 4.86ms | 43.49KB | 0 |
| dynamic-ssz (reflection) | UnmarshalReader | 3.55ms | 2.93MB | 12722 |
| dynamic-ssz (reflection) | MarshalWriter | 1.07ms | 92.94KB | 11493 |
| karalabe-ssz | Unmarshal | 1.42ms | 2.83MB | 602 |
| karalabe-ssz | Marshal | 1.16ms | 2.81MB | 2 |
| karalabe-ssz | HashTreeRoot | 3.57ms | 20B | 0 |
| karalabe-ssz | UnmarshalReader | 2.02ms | 2.83MB | 603 |
| karalabe-ssz | MarshalWriter | 321.06µs | 1B | 0 |

### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v2) | Unmarshal | 4.33µs | 3.09KB | 51 |
| fastssz (v2) | Marshal | 1.40µs | 2.05KB | 1 |
| fastssz (v2) | HashTreeRoot | 10.26µs | 0B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 2.42µs | 2.60KB | 29 |
| dynamic-ssz (codegen) | Marshal | 1.50µs | 2.05KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 6.60µs | 320B | 12 |
| dynamic-ssz (reflection) | Unmarshal | 7.65µs | 3.47KB | 65 |
| dynamic-ssz (reflection) | Marshal | 4.17µs | 2.05KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 7.49µs | 320B | 12 |
| dynamic-ssz (reflection) | UnmarshalReader | 12.77µs | 5.91KB | 121 |
| dynamic-ssz (reflection) | MarshalWriter | 3.94µs | 1.34KB | 47 |

### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
| fastssz (v2) | Unmarshal | 98.56µs | 79.87KB | 698 |
| fastssz (v2) | Marshal | 41.01µs | 73.73KB | 1 |
| fastssz (v2) | HashTreeRoot | 309.86µs | 13B | 0 |
| dynamic-ssz (codegen) | Unmarshal | 55.07µs | 72.39KB | 430 |
| dynamic-ssz (codegen) | Marshal | 41.52µs | 73.73KB | 1 |
| dynamic-ssz (codegen) | HashTreeRoot | 180.22µs | 5B | 0 |
| dynamic-ssz (reflection) | Unmarshal | 177.80µs | 82.86KB | 861 |
| dynamic-ssz (reflection) | Marshal | 146.06µs | 73.73KB | 1 |
| dynamic-ssz (reflection) | HashTreeRoot | 216.25µs | 9B | 0 |
| dynamic-ssz (reflection) | UnmarshalReader | 223.38µs | 110.13KB | 4055 |
| dynamic-ssz (reflection) | MarshalWriter | 90.72µs | 26.42KB | 3178 |

**Note:** karalabe-ssz does not support minimal preset out of the box.

<!-- BENCHMARK_RESULTS_END -->

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
