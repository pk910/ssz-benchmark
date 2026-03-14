package prysmssz

//go:generate go run github.com/prysmaticlabs/fastssz/sszgen@v0.0.0-20260313210737-46c2c806e3e1 --output gen_ssz.go --path types.go --objs Fork,Checkpoint,BeaconBlockHeader,SignedBeaconBlockHeader,ETH1Data,Validator,ProposerSlashing,AttestationData,IndexedAttestation,AttesterSlashing,Attestation,DepositData,Deposit,VoluntaryExit,SignedVoluntaryExit,SyncAggregate,SyncCommittee,Withdrawal,BLSToExecutionChange,SignedBLSToExecutionChange,HistoricalSummary,ExecutionPayload,ExecutionPayloadHeader,BeaconBlockBody,BeaconBlock,SignedBeaconBlock,BeaconState
