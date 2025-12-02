package karalabessz

// Basic types first
//go:generate sszgen -type Checkpoint -out gen_checkpoint_ssz.go
//go:generate sszgen -type Fork -out gen_fork_ssz.go
//go:generate sszgen -type BeaconBlockHeader -out gen_beacon_block_header_ssz.go
//go:generate sszgen -type Eth1Data -out gen_eth1_data_ssz.go
//go:generate sszgen -type DepositData -out gen_deposit_data_ssz.go
//go:generate sszgen -type VoluntaryExit -out gen_voluntary_exit_ssz.go
//go:generate sszgen -type Validator -out gen_validator_ssz.go
//go:generate sszgen -type Withdrawal -out gen_withdrawal_ssz.go
//go:generate sszgen -type BLSToExecutionChange -out gen_bls_to_execution_change_ssz.go
//go:generate sszgen -type HistoricalSummary -out gen_historical_summary_ssz.go
//go:generate sszgen -type SyncAggregate -out gen_sync_aggregate_ssz.go
//go:generate sszgen -type SyncCommittee -out gen_sync_committee_ssz.go

// Types depending on basic types
//go:generate sszgen -type SignedBeaconBlockHeader -out gen_signed_beacon_block_header_ssz.go
//go:generate sszgen -type AttestationData -out gen_attestation_data_ssz.go
//go:generate sszgen -type Deposit -out gen_deposit_ssz.go
//go:generate sszgen -type SignedVoluntaryExit -out gen_signed_voluntary_exit_ssz.go
//go:generate sszgen -type SignedBLSToExecutionChange -out gen_signed_bls_to_execution_change_ssz.go

// Types depending on the above
//go:generate sszgen -type IndexedAttestation -out gen_indexed_attestation_ssz.go
//go:generate sszgen -type Attestation -out gen_attestation_ssz.go
//go:generate sszgen -type ProposerSlashing -out gen_proposer_slashing_ssz.go
//go:generate sszgen -type ExecutionPayloadHeaderDeneb -out gen_execution_payload_header_deneb_ssz.go

// Types depending on further types
//go:generate sszgen -type AttesterSlashing -out gen_attester_slashing_ssz.go
//go:generate sszgen -type ExecutionPayloadDeneb -out gen_execution_payload_deneb_ssz.go

// Block and state types (depend on many others)
//go:generate sszgen -type BeaconBlockBodyDeneb -out gen_beacon_block_body_deneb_ssz.go
//go:generate sszgen -type BeaconBlockDeneb -out gen_beacon_block_deneb_ssz.go
//go:generate sszgen -type SignedBeaconBlockDeneb -out gen_signed_beacon_block_deneb_ssz.go
//go:generate sszgen -type BeaconStateDeneb -out gen_beacon_state_deneb_ssz.go
