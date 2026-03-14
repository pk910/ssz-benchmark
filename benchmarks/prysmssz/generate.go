package prysmssz

//go:generate go run github.com/prysmaticlabs/fastssz/sszgen@v0.0.0-20251103153600-259302269bfc --output gen_ssz.go --path types.go --objs Fork,Checkpoint,BeaconBlockHeader,SignedBeaconBlockHeader,ETH1Data,Validator,ProposerSlashing,AttestationData,IndexedAttestation,AttesterSlashing,Attestation,DepositData,Deposit,VoluntaryExit,SignedVoluntaryExit,SyncAggregate,SyncCommittee,Withdrawal,BLSToExecutionChange,SignedBLSToExecutionChange,HistoricalSummary,ExecutionPayload,ExecutionPayloadHeader,BeaconBlockBody,BeaconBlock,SignedBeaconBlock,BeaconState
