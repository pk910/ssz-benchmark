package fastssz

//go:generate go run github.com/ferranbt/fastssz/sszgen@v1.0.0 --path . --objs Fork,Checkpoint,BeaconBlockHeader,SignedBeaconBlockHeader,ETH1Data,Validator,ProposerSlashing,AttestationData,IndexedAttestation,AttesterSlashing,Attestation,DepositData,Deposit,VoluntaryExit,SignedVoluntaryExit,SyncAggregate,SyncCommittee,Withdrawal,BLSToExecutionChange,SignedBLSToExecutionChange,HistoricalSummary,ExecutionPayload,ExecutionPayloadHeader,BeaconBlockBody,BeaconBlock,SignedBeaconBlock,BeaconState
