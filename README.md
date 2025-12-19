# SSZ Benchmark Suite

A comprehensive benchmarking suite for comparing SSZ (Simple Serialize) library implementations in Go.

## Libraries Tested

- **[fastssz](https://github.com/ferranbt/fastssz)** - Code generation based SSZ library
- **[dynamic-ssz](https://github.com/pk910/dynamic-ssz)** - Dynamic SSZ library with support for both reflection and code generation modes
- **[karalabe-ssz](https://github.com/karalabe/ssz)** - High-performance SSZ library
- **[ztyp](https://github.com/protolambda/ztyp)** / **[zrnt](https://github.com/protolambda/zrnt)** - Typed SSZ library focused on merkle-tree representations (uses zrnt's pre-defined Ethereum types)

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

# Run ztyp/zrnt benchmarks
cd benchmarks/ztyp
go test -run=^$ -bench=. -benchmem
```

## Project Structure

```
ssz-benchmark/
├── benchmarks/
│   ├── fastssz/              # fastssz benchmark module
│   ├── dynamicssz/           # dynamic-ssz with generated code
│   ├── dynamicssz-reflection/# dynamic-ssz pure reflection (no codegen)
│   ├── karalabessz/          # karalabe-ssz benchmark module
│   └── ztyp/                 # ztyp/zrnt benchmark module
├── res/                      # Test data files
│   ├── block-mainnet.ssz
│   ├── state-mainnet.ssz
│   ├── block-minimal.ssz
│   └── state-minimal.ssz
└── .github/workflows/        # CI/CD workflows
```

## Benchmark Results

![SSZ Benchmark Results](https://pk910.github.io/ssz-benchmark/benchmark-table.svg)

![SSZ Benchmark Charts](https://pk910.github.io/ssz-benchmark/benchmark-charts.svg)

View interactive benchmark results and historical trends at: https://pk910.github.io/ssz-benchmark/

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues for:
- Adding new SSZ libraries
- Improving benchmark methodology
- Adding new test scenarios

## License

MIT License
