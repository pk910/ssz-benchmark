package fastssz

//go:generate go run github.com/ferranbt/fastssz/sszgen@v0.0.0-20250808103907-ac370aa5f7e4 --output gen_ssz.go --path . --objs Fork,Checkpoint,BeaconBlockHeader,SignedBeaconBlockHeader,ETH1Data,Validator,ProposerSlashing,AttestationData,IndexedAttestation,AttesterSlashing,Attestation,DepositData,Deposit,VoluntaryExit,SignedVoluntaryExit,SyncAggregate,SyncCommittee,Withdrawal,BLSToExecutionChange,SignedBLSToExecutionChange,HistoricalSummary,ExecutionPayload,ExecutionPayloadHeader,BeaconBlockBody,BeaconBlock,SignedBeaconBlock,BeaconState
