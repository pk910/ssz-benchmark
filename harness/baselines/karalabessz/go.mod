module baseline/karalabessz

go 1.26.0

require (
	benchkit v0.0.0
	github.com/holiman/uint256 v1.3.2
	github.com/karalabe/ssz v0.3.0
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15
)

require (
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/prysmaticlabs/gohashtree v0.0.4-beta // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace benchkit => ../../kit

replace github.com/karalabe/ssz => ./ssz
