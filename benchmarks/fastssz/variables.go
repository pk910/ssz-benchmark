package fastssz

// Spec variables for dynamic SSZ sizing
// These are used by the generated SSZ code via var() syntax
var (
	// SLOTS_PER_HISTORICAL_ROOT
	slotsPerHistoricalRoot uint64

	// EPOCHS_PER_ETH1_VOTING_PERIOD * SLOTS_PER_EPOCH
	eth1DataVotesLimit uint64

	// EPOCHS_PER_HISTORICAL_VECTOR
	epochsPerHistoricalVector uint64

	// EPOCHS_PER_SLASHINGS_VECTOR
	epochsPerSlashingsVector uint64

	// SYNC_COMMITTEE_SIZE / 8 (bytes)
	syncCommitteeBitsSize uint64

	// SYNC_COMMITTEE_SIZE
	syncCommitteeSize uint64

	// MAX_WITHDRAWALS_PER_PAYLOAD
	maxWithdrawals uint64

	// MAX_BLOB_COMMITMENTS_PER_BLOCK
	maxBlobCommitmentsPerBlock uint64
)

func init() {
	SetMainnetSpec()
}

// SetMainnetSpec sets the spec variables to mainnet values
func SetMainnetSpec() {
	slotsPerHistoricalRoot = 8192
	eth1DataVotesLimit = 2048
	epochsPerHistoricalVector = 65536
	epochsPerSlashingsVector = 8192
	syncCommitteeBitsSize = 64
	syncCommitteeSize = 512
	maxWithdrawals = 16
	maxBlobCommitmentsPerBlock = 4096
}

// SetMinimalSpec sets the spec variables to minimal values
func SetMinimalSpec() {
	slotsPerHistoricalRoot = 64
	eth1DataVotesLimit = 32
	epochsPerHistoricalVector = 64
	epochsPerSlashingsVector = 64
	syncCommitteeBitsSize = 4
	syncCommitteeSize = 32
	maxWithdrawals = 4
	maxBlobCommitmentsPerBlock = 32
}
