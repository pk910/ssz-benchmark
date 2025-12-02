package karalabessz

import (
	"github.com/holiman/uint256"
	"github.com/prysmaticlabs/go-bitfield"
)

// Slot is an alias of uint64
type Slot uint64

// Hash is a standalone mock of go-ethereum's common.Hash
type Hash [32]byte

// Address is a standalone mock of go-ethereum's common.Address
type Address [20]byte

// LogsBloom is a standalone mock of go-ethereum's types.LogsBloom
type LogsBloom [256]byte

// Fork represents a fork
type Fork struct {
	PreviousVersion [4]byte
	CurrentVersion  [4]byte
	Epoch           uint64
}

// Checkpoint represents a checkpoint
type Checkpoint struct {
	Epoch uint64
	Root  Hash
}

// BeaconBlockHeader represents a beacon block header
type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    Hash
	StateRoot     Hash
	BodyRoot      Hash
}

// SignedBeaconBlockHeader represents a signed beacon block header
type SignedBeaconBlockHeader struct {
	Header    *BeaconBlockHeader
	Signature [96]byte
}

// Eth1Data represents eth1 data
type Eth1Data struct {
	DepositRoot  Hash
	DepositCount uint64
	BlockHash    Hash
}

// Validator represents a validator
type Validator struct {
	Pubkey                     [48]byte
	WithdrawalCredentials      [32]byte
	EffectiveBalance           uint64
	Slashed                    bool
	ActivationEligibilityEpoch uint64
	ActivationEpoch            uint64
	ExitEpoch                  uint64
	WithdrawableEpoch          uint64
}

// ProposerSlashing represents a proposer slashing
type ProposerSlashing struct {
	Header1 *SignedBeaconBlockHeader
	Header2 *SignedBeaconBlockHeader
}

// AttestationData represents attestation data
type AttestationData struct {
	Slot            Slot
	Index           uint64
	BeaconBlockHash Hash
	Source          *Checkpoint
	Target          *Checkpoint
}

// IndexedAttestation represents an indexed attestation
type IndexedAttestation struct {
	AttestationIndices []uint64 `ssz-max:"2048"`
	Data               *AttestationData
	Signature          [96]byte
}

// AttesterSlashing represents an attester slashing
type AttesterSlashing struct {
	Attestation1 *IndexedAttestation
	Attestation2 *IndexedAttestation
}

// Attestation represents an attestation
type Attestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"2048"`
	Data            *AttestationData
	Signature       [96]byte
}

// DepositData represents deposit data
type DepositData struct {
	Pubkey                [48]byte
	WithdrawalCredentials [32]byte
	Amount                uint64
	Signature             [96]byte
}

// Deposit represents a deposit
type Deposit struct {
	Proof [33][32]byte
	Data  *DepositData
}

// VoluntaryExit represents a voluntary exit
type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

// SignedVoluntaryExit represents a signed voluntary exit
type SignedVoluntaryExit struct {
	Exit      *VoluntaryExit
	Signature [96]byte
}

// SyncAggregate represents a sync aggregate
type SyncAggregate struct {
	SyncCommiteeBits      [64]byte
	SyncCommiteeSignature [96]byte
}

// SyncCommittee represents a sync committee
type SyncCommittee struct {
	PubKeys         [512][48]byte
	AggregatePubKey [48]byte
}

// Withdrawal represents a withdrawal
type Withdrawal struct {
	Index     uint64
	Validator uint64
	Address   Address
	Amount    uint64
}

// BLSToExecutionChange represents a BLS to execution change
type BLSToExecutionChange struct {
	ValidatorIndex     uint64
	FromBLSPubKey      [48]byte
	ToExecutionAddress [20]byte
}

// SignedBLSToExecutionChange represents a signed BLS to execution change
type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange
	Signature [96]byte
}

// HistoricalSummary represents a historical summary
type HistoricalSummary struct {
	BlockSummaryRoot [32]byte
	StateSummaryRoot [32]byte
}

// ExecutionPayloadDeneb represents an execution payload (Deneb)
type ExecutionPayloadDeneb struct {
	ParentHash    Hash
	FeeRecipient  Address
	StateRoot     Hash
	ReceiptsRoot  Hash
	LogsBloom     LogsBloom
	PrevRandao    Hash
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte `ssz-max:"32"`
	BaseFeePerGas *uint256.Int
	BlockHash     Hash
	Transactions  [][]byte      `ssz-max:"1048576,1073741824"`
	Withdrawals   []*Withdrawal `ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

// ExecutionPayloadHeaderDeneb represents an execution payload header (Deneb)
type ExecutionPayloadHeaderDeneb struct {
	ParentHash       [32]byte
	FeeRecipient     [20]byte
	StateRoot        [32]byte
	ReceiptsRoot     [32]byte
	LogsBloom        [256]byte
	PrevRandao       [32]byte
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte `ssz-max:"32"`
	BaseFeePerGas    [32]byte
	BlockHash        [32]byte
	TransactionsRoot [32]byte
	WithdrawalRoot   [32]byte
	BlobGasUsed      uint64
	ExcessBlobGas    uint64
}

// BeaconBlockBodyDeneb represents a beacon block body (Deneb)
type BeaconBlockBodyDeneb struct {
	RandaoReveal          [96]byte
	Eth1Data              *Eth1Data
	Graffiti              [32]byte
	ProposerSlashings     []*ProposerSlashing           `ssz-max:"16"`
	AttesterSlashings     []*AttesterSlashing           `ssz-max:"2"`
	Attestations          []*Attestation                `ssz-max:"128"`
	Deposits              []*Deposit                    `ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit        `ssz-max:"16"`
	SyncAggregate         *SyncAggregate
	ExecutionPayload      *ExecutionPayloadDeneb
	BlsToExecutionChanges []*SignedBLSToExecutionChange `ssz-max:"16"`
	BlobKzgCommitments    [][48]byte                    `ssz-max:"4096"`
}

// BeaconBlockDeneb represents a beacon block (Deneb)
type BeaconBlockDeneb struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Hash
	StateRoot     Hash
	Body          *BeaconBlockBodyDeneb
}

// SignedBeaconBlockDeneb represents a signed beacon block (Deneb)
type SignedBeaconBlockDeneb struct {
	Message   *BeaconBlockDeneb
	Signature [96]byte
}

// BeaconStateDeneb represents a beacon state (Deneb)
type BeaconStateDeneb struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        [32]byte
	Slot                         uint64
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   [8192][32]byte
	StateRoots                   [8192][32]byte
	HistoricalRoots              [][32]byte           `ssz-max:"16777216"`
	Eth1Data                     *Eth1Data
	Eth1DataVotes                []*Eth1Data          `ssz-max:"2048"`
	Eth1DepositIndex             uint64
	Validators                   []*Validator         `ssz-max:"1099511627776"`
	Balances                     []uint64             `ssz-max:"1099511627776"`
	RandaoMixes                  [65536][32]byte
	Slashings                    [8192]uint64
	PreviousEpochParticipation   []byte               `ssz-max:"1099511627776"`
	CurrentEpochParticipation    []byte               `ssz-max:"1099511627776"`
	JustificationBits            [1]byte              `ssz-size:"4" ssz:"bits"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64             `ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *ExecutionPayloadHeaderDeneb
	NextWithdrawalIndex          uint64
	NextWithdrawalValidatorIndex uint64
	HistoricalSummaries          []*HistoricalSummary `ssz-max:"16777216"`
}
