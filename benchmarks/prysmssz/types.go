package prysmssz

import (
	"github.com/prysmaticlabs/go-bitfield"
)

// Fork represents a fork
type Fork struct {
	PreviousVersion [4]byte `ssz-size:"4"`
	CurrentVersion  [4]byte `ssz-size:"4"`
	Epoch           uint64
}

// Checkpoint represents a checkpoint
type Checkpoint struct {
	Epoch uint64
	Root  [32]byte `ssz-size:"32"`
}

// BeaconBlockHeader represents a beacon block header
type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	BodyRoot      [32]byte `ssz-size:"32"`
}

// SignedBeaconBlockHeader represents a signed beacon block header
type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature [96]byte `ssz-size:"96"`
}

// ETH1Data represents eth1 data
type ETH1Data struct {
	DepositRoot  [32]byte `ssz-size:"32"`
	DepositCount uint64
	BlockHash    [32]byte `ssz-size:"32"`
}

// Validator represents a validator
type Validator struct {
	Pubkey                     [48]byte `ssz-size:"48"`
	WithdrawalCredentials      [32]byte `ssz-size:"32"`
	EffectiveBalance           uint64
	Slashed                    bool
	ActivationEligibilityEpoch uint64
	ActivationEpoch            uint64
	ExitEpoch                  uint64
	WithdrawableEpoch          uint64
}

// ProposerSlashing represents a proposer slashing
type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader
	SignedHeader2 *SignedBeaconBlockHeader
}

// AttestationData represents attestation data
type AttestationData struct {
	Slot            uint64
	Index           uint64
	BeaconBlockRoot [32]byte `ssz-size:"32"`
	Source          *Checkpoint
	Target          *Checkpoint
}

// IndexedAttestation represents an indexed attestation
type IndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"2048"`
	Data             *AttestationData
	Signature        [96]byte `ssz-size:"96"`
}

// AttesterSlashing represents an attester slashing
type AttesterSlashing struct {
	Attestation1 *IndexedAttestation
	Attestation2 *IndexedAttestation
}

// Attestation represents an attestation
type Attestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"2048" ssz:"bitlist"`
	Data            *AttestationData
	Signature       [96]byte `ssz-size:"96"`
}

// DepositData represents deposit data
type DepositData struct {
	Pubkey                [48]byte `ssz-size:"48"`
	WithdrawalCredentials [32]byte `ssz-size:"32"`
	Amount                uint64
	Signature             [96]byte `ssz-size:"96"`
}

// Deposit represents a deposit
type Deposit struct {
	Proof [][]byte `ssz-size:"33,32"`
	Data  *DepositData
}

// VoluntaryExit represents a voluntary exit
type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

// SignedVoluntaryExit represents a signed voluntary exit
type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature [96]byte `ssz-size:"96"`
}

// SyncAggregate represents a sync aggregate
type SyncAggregate struct {
	SyncCommitteeBits      []byte   `ssz-size:"64"`
	SyncCommitteeSignature [96]byte `ssz-size:"96"`
}

// SyncCommittee represents a sync committee
type SyncCommittee struct {
	Pubkeys         [][]byte `ssz-size:"512,48"`
	AggregatePubkey [48]byte `ssz-size:"48"`
}

// Withdrawal represents a withdrawal
type Withdrawal struct {
	Index          uint64
	ValidatorIndex uint64
	Address        [20]byte `ssz-size:"20"`
	Amount         uint64
}

// BLSToExecutionChange represents a BLS to execution change
type BLSToExecutionChange struct {
	ValidatorIndex     uint64
	FromBLSPubkey      [48]byte `ssz-size:"48"`
	ToExecutionAddress [20]byte `ssz-size:"20"`
}

// SignedBLSToExecutionChange represents a signed BLS to execution change
type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange
	Signature [96]byte `ssz-size:"96"`
}

// HistoricalSummary represents a historical summary
type HistoricalSummary struct {
	BlockSummaryRoot [32]byte `ssz-size:"32"`
	StateSummaryRoot [32]byte `ssz-size:"32"`
}

// ExecutionPayload represents an execution payload (Deneb)
type ExecutionPayload struct {
	ParentHash    [32]byte  `ssz-size:"32"`
	FeeRecipient  [20]byte  `ssz-size:"20"`
	StateRoot     [32]byte  `ssz-size:"32"`
	ReceiptsRoot  [32]byte  `ssz-size:"32"`
	LogsBloom     [256]byte `ssz-size:"256"`
	PrevRandao    [32]byte  `ssz-size:"32"`
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte        `ssz-max:"32"`
	BaseFeePerGas [32]byte      `ssz-size:"32"`
	BlockHash     [32]byte      `ssz-size:"32"`
	Transactions  [][]byte      `ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals   []*Withdrawal `ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

// ExecutionPayloadHeader represents an execution payload header (Deneb)
type ExecutionPayloadHeader struct {
	ParentHash       [32]byte  `ssz-size:"32"`
	FeeRecipient     [20]byte  `ssz-size:"20"`
	StateRoot        [32]byte  `ssz-size:"32"`
	ReceiptsRoot     [32]byte  `ssz-size:"32"`
	LogsBloom        [256]byte `ssz-size:"256"`
	PrevRandao       [32]byte  `ssz-size:"32"`
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte   `ssz-max:"32"`
	BaseFeePerGas    [32]byte `ssz-size:"32"`
	BlockHash        [32]byte `ssz-size:"32"`
	TransactionsRoot [32]byte `ssz-size:"32"`
	WithdrawalsRoot  [32]byte `ssz-size:"32"`
	BlobGasUsed      uint64
	ExcessBlobGas    uint64
}

// BeaconBlockBody represents a beacon block body (Deneb)
type BeaconBlockBody struct {
	RANDAOReveal          [96]byte `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              [32]byte               `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing    `ssz-max:"16"`
	AttesterSlashings     []*AttesterSlashing    `ssz-max:"2"`
	Attestations          []*Attestation         `ssz-max:"128"`
	Deposits              []*Deposit             `ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit `ssz-max:"16"`
	SyncAggregate         *SyncAggregate
	ExecutionPayload      *ExecutionPayload
	BLSToExecutionChanges []*SignedBLSToExecutionChange `ssz-max:"16"`
	BlobKZGCommitments    [][48]byte                    `ssz-max:"4096" ssz-size:"?,48"`
}

// BeaconBlock represents a beacon block (Deneb)
type BeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	Body          *BeaconBlockBody
}

// SignedBeaconBlock represents a signed beacon block (Deneb)
type SignedBeaconBlock struct {
	Message   *BeaconBlock
	Signature [96]byte `ssz-size:"96"`
}

// BeaconState represents a beacon state (Deneb)
type BeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        [32]byte `ssz-size:"32"`
	Slot                         uint64
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   [][]byte   `ssz-size:"8192,32"`
	StateRoots                   [][]byte   `ssz-size:"8192,32"`
	HistoricalRoots              [][32]byte `ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *ETH1Data
	ETH1DataVotes                []*ETH1Data `ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Validator `ssz-max:"1099511627776"`
	Balances                     []uint64     `ssz-max:"1099511627776"`
	RANDAOMixes                  [][]byte     `ssz-size:"65536,32"`
	Slashings                    []uint64     `ssz-size:"8192"`
	PreviousEpochParticipation   []uint8      `ssz-max:"1099511627776"`
	CurrentEpochParticipation    []uint8      `ssz-max:"1099511627776"`
	JustificationBits            [1]byte      `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *ExecutionPayloadHeader
	NextWithdrawalIndex          uint64
	NextWithdrawalValidatorIndex uint64
	HistoricalSummaries          []*HistoricalSummary `ssz-max:"16777216"`
}
