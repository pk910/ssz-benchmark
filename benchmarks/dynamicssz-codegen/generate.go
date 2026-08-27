package dynamicssz

//go:generate go run github.com/pk910/dynamic-ssz/dynssz-gen@v1.3.3 -package . -types SignedBeaconBlock,BeaconBlock,BeaconState -output gen_ssz.go -legacy -with-streaming
