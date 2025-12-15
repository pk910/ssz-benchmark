package dynamicssz

//go:generate go run github.com/pk910/dynamic-ssz/dynssz-gen@v1.1.2 -package . -types SignedBeaconBlock,BeaconBlock,BeaconState -output gen_ssz.go -legacy
