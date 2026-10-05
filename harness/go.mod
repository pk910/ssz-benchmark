module realbench

go 1.26.0

require (
	benchkit v0.0.0-00010101000000-000000000000
	github.com/holiman/uint256 v1.3.2
	github.com/pk910/dynamic-ssz v1.3.2
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15
	golang.org/x/sys v0.48.0
)

require (
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/pk910/hashtree-bindings v0.2.6 // indirect
)

replace github.com/pk910/dynamic-ssz => ../../dynamic-ssz

replace benchkit => ./kit
