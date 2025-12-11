#!/bin/sh

# we need to set the GOTOOLCHAIN to go1.23.4
# karalabe/ssz is incompatible with newer versions of Go due to its outdated `golang.org/x/tool` import
export GOTOOLCHAIN=go1.23.4

# replace go version in go.mod to 1.23.4
cp go.mod go.mod.tmp
sed -i 's/go 1.25.0/go 1.23.4/' go.mod
trap 'mv go.mod.tmp go.mod' EXIT

go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Checkpoint -out gen_checkpoint_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Fork -out gen_fork_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type BeaconBlockHeader -out gen_beacon_block_header_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Eth1Data -out gen_eth1_data_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type DepositData -out gen_deposit_data_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type VoluntaryExit -out gen_voluntary_exit_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Validator -out gen_validator_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Withdrawal -out gen_withdrawal_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type BLSToExecutionChange -out gen_bls_to_execution_change_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type HistoricalSummary -out gen_historical_summary_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SyncAggregate -out gen_sync_aggregate_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SyncCommittee -out gen_sync_committee_ssz.go

# Types depending on basic types
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SignedBeaconBlockHeader -out gen_signed_beacon_block_header_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type AttestationData -out gen_attestation_data_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Deposit -out gen_deposit_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SignedVoluntaryExit -out gen_signed_voluntary_exit_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SignedBLSToExecutionChange -out gen_signed_bls_to_execution_change_ssz.go

# Types depending on the above
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type IndexedAttestation -out gen_indexed_attestation_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type Attestation -out gen_attestation_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type ProposerSlashing -out gen_proposer_slashing_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type ExecutionPayloadHeaderDeneb -out gen_execution_payload_header_deneb_ssz.go

# Types depending on further types
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type AttesterSlashing -out gen_attester_slashing_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type ExecutionPayloadDeneb -out gen_execution_payload_deneb_ssz.go

# Block and state types (depend on many others)
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type BeaconBlockBodyDeneb -out gen_beacon_block_body_deneb_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type BeaconBlockDeneb -out gen_beacon_block_deneb_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type SignedBeaconBlockDeneb -out gen_signed_beacon_block_deneb_ssz.go
go run github.com/karalabe/ssz/cmd/sszgen@v0.3.0 -type BeaconStateDeneb -out gen_beacon_state_deneb_ssz.go