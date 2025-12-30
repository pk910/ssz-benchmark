package dynamicssz

//go:generate go run github.com/pk910/dynamic-ssz/dynssz-gen@v0.0.0-20251230180814-9edadc012644 -package . -types SignedBeaconBlock,BeaconBlock,BeaconState -output gen_ssz.go -legacy
