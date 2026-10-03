package fulu

import (
	"github.com/prysmaticlabs/go-bitfield"
)

// base types

// Phase0 types

type AttestationData struct {
	Slot            uint64
	Index           uint64
	BeaconBlockRoot [32]byte `ssz-size:"32"`
	Source          *Checkpoint
	Target          *Checkpoint
}

type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	BodyRoot      [32]byte `ssz-size:"32"`
}

type Checkpoint struct {
	Epoch uint64
	Root  [32]byte `ssz-size:"32"`
}

type Deposit struct {
	Proof [][]byte `ssz-size:"33,32"`
	Data  *DepositData
}

type DepositData struct {
	PublicKey             [48]byte `ssz-size:"48"`
	WithdrawalCredentials []byte   `ssz-size:"32"`
	Amount                uint64
	Signature             [96]byte `ssz-size:"96"`
}

type ETH1Data struct {
	DepositRoot  [32]byte `ssz-size:"32"`
	DepositCount uint64
	BlockHash    []byte `ssz-size:"32"`
}

type Fork struct {
	PreviousVersion [4]byte `ssz-size:"4"`
	CurrentVersion  [4]byte `ssz-size:"4"`
	Epoch           uint64
}

type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader
	SignedHeader2 *SignedBeaconBlockHeader
}

type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature [96]byte `ssz-size:"96"`
}

type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature [96]byte `ssz-size:"96"`
}

type Validator struct {
	PublicKey                  [48]byte `ssz-size:"48"`
	WithdrawalCredentials      []byte   `ssz-size:"32"`
	EffectiveBalance           uint64
	Slashed                    bool
	ActivationEligibilityEpoch uint64
	ActivationEpoch            uint64
	ExitEpoch                  uint64
	WithdrawableEpoch          uint64
}

type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

// Altair types

type AltairSyncAggregate struct {
	SyncCommitteeBits      [64]byte `ssz-size:"64"`
	SyncCommitteeSignature [96]byte `ssz-size:"96"`
}

type AltairSyncCommittee struct {
	Pubkeys         [][48]byte `ssz-size:"512,48"`
	AggregatePubkey [48]byte   `ssz-size:"48"`
}

// Bellatrix types

// Capella types

type CapellaBLSToExecutionChange struct {
	ValidatorIndex     uint64
	FromBLSPubkey      [48]byte `ssz-size:"48"`
	ToExecutionAddress [20]byte `ssz-size:"20"`
}

type CapellaHistoricalSummary struct {
	BlockSummaryRoot [32]byte `ssz-size:"32"`
	StateSummaryRoot [32]byte `ssz-size:"32"`
}

type CapellaSignedBLSToExecutionChange struct {
	Message   *CapellaBLSToExecutionChange
	Signature [96]byte `ssz-size:"96"`
}

type CapellaWithdrawal struct {
	Index          uint64
	ValidatorIndex uint64
	Address        [20]byte `ssz-size:"20"`
	Amount         uint64
}

// Deneb types

type DenebExecutionPayload struct {
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
	ExtraData     []byte               `ssz-max:"32"`
	BaseFeePerGas [32]byte             `ssz-size:"32"`
	BlockHash     [32]byte             `ssz-size:"32"`
	Transactions  [][]byte             `ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals   []*CapellaWithdrawal `ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

type DenebExecutionPayloadHeader struct {
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

// Electra types

type ElectraAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"131072" ssz:"bitlist"`
	Data            *AttestationData
	Signature       [96]byte `ssz-size:"96"`
	CommitteeBits   [8]byte  `ssz-size:"8"`
}

type ElectraAttesterSlashing struct {
	Attestation1 *ElectraIndexedAttestation
	Attestation2 *ElectraIndexedAttestation
}

type ElectraBeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	Body          *ElectraBeaconBlockBody
}

type ElectraBeaconBlockBody struct {
	RANDAOReveal          [96]byte `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              [32]byte                   `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing        `ssz-max:"16"`
	AttesterSlashings     []*ElectraAttesterSlashing `ssz-max:"1"`
	Attestations          []*ElectraAttestation      `ssz-max:"8"`
	Deposits              []*Deposit                 `ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit     `ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *DenebExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `ssz-max:"16"`
	BlobKZGCommitments    [][48]byte                           `ssz-max:"4096" ssz-size:"?,48"`
	ExecutionRequests     *ElectraExecutionRequests
}

type ElectraConsolidationRequest struct {
	SourceAddress [20]byte `ssz-size:"20"`
	SourcePubkey  [48]byte `ssz-size:"48"`
	TargetPubkey  [48]byte `ssz-size:"48"`
}

type ElectraDepositRequest struct {
	Pubkey                [48]byte `ssz-size:"48"`
	WithdrawalCredentials []byte   `ssz-size:"32"`
	Amount                uint64
	Signature             [96]byte `ssz-size:"96"`
	Index                 uint64
}

type ElectraExecutionRequests struct {
	Deposits       []*ElectraDepositRequest       `ssz-max:"8192"`
	Withdrawals    []*ElectraWithdrawalRequest    `ssz-max:"16"`
	Consolidations []*ElectraConsolidationRequest `ssz-max:"2"`
}

type ElectraIndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"131072"`
	Data             *AttestationData
	Signature        [96]byte `ssz-size:"96"`
}

type ElectraPendingDeposit struct {
	Pubkey                [48]byte `ssz-size:"48"`
	WithdrawalCredentials []byte   `ssz-size:"32"`
	Amount                uint64
	Signature             [96]byte `ssz-size:"96"`
	Slot                  uint64
}

type ElectraPendingConsolidation struct {
	SourceIndex uint64
	TargetIndex uint64
}

type ElectraPendingPartialWithdrawal struct {
	ValidatorIndex    uint64
	Amount            uint64
	WithdrawableEpoch uint64
}

type ElectraSignedBeaconBlock struct {
	Message   *ElectraBeaconBlock
	Signature [96]byte `ssz-size:"96"`
}

type ElectraWithdrawalRequest struct {
	SourceAddress   [20]byte `ssz-size:"20"`
	ValidatorPubkey [48]byte `ssz-size:"48"`
	Amount          uint64
}

// Fulu types
type FuluBeaconState struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         [32]byte `ssz-size:"32"`
	Slot                          uint64
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    [][32]byte `ssz-size:"8192,32"`
	StateRoots                    [][32]byte `ssz-size:"8192,32"`
	HistoricalRoots               [][32]byte `ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *ETH1Data
	ETH1DataVotes                 []*ETH1Data `ssz-max:"2048"`
	ETH1DepositIndex              uint64
	Validators                    []*Validator `ssz-max:"1099511627776"`
	Balances                      []uint64     `ssz-max:"1099511627776"`
	RANDAOMixes                   [][32]byte   `ssz-size:"65536,32"`
	Slashings                     []uint64     `ssz-size:"8192"`
	PreviousEpochParticipation    []uint8      `ssz-max:"1099511627776"`
	CurrentEpochParticipation     []uint8      `ssz-max:"1099511627776"`
	JustificationBits             [1]byte      `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee          *AltairSyncCommittee
	NextSyncCommittee             *AltairSyncCommittee
	LatestExecutionPayloadHeader  *DenebExecutionPayloadHeader
	NextWithdrawalIndex           uint64
	NextWithdrawalValidatorIndex  uint64
	HistoricalSummaries           []*CapellaHistoricalSummary `ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       uint64
	ExitBalanceToConsume          uint64
	EarliestExitEpoch             uint64
	ConsolidationBalanceToConsume uint64
	EarliestConsolidationEpoch    uint64
	PendingDeposits               []*ElectraPendingDeposit           `ssz-max:"134217728"`
	PendingPartialWithdrawals     []*ElectraPendingPartialWithdrawal `ssz-max:"134217728"`
	PendingConsolidations         []*ElectraPendingConsolidation     `ssz-max:"262144"`
	ProposerLookahead             []uint64                           `ssz-size:"64"`
}
