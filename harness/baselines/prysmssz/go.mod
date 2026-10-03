module baseline/prysmssz

go 1.26.0

require (
	benchkit v0.0.0
	github.com/OffchainLabs/go-bitfield v0.0.0-20260504143531-5cbb6d0f5f2e
	github.com/OffchainLabs/methodical-ssz v0.0.0-20260703104215-9be4f5c6a334
)

require (
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/prysmaticlabs/gohashtree v0.0.5-beta // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace benchkit => ../../kit
