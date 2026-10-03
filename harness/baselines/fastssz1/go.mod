module baseline/fastssz1

go 1.26.0

require (
	benchkit v0.0.0
	github.com/ferranbt/fastssz v1.0.0
	github.com/prysmaticlabs/go-bitfield v0.0.0-20240618144021-706c95b2dd15
)

require (
	github.com/emicklei/dot v1.6.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.3.2 // indirect
	golang.org/x/sys v0.48.0 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
)

replace benchkit => ../../kit
