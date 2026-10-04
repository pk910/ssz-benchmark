package fulu

import (
	"github.com/holiman/uint256"
	"github.com/prysmaticlabs/go-bitfield"
)

// base types

// Phase0 types

type AttestationData struct {
	Slot            uint64
	Index           uint64
	BeaconBlockRoot [32]byte
	Source          *Checkpoint
	Target          *Checkpoint
}

type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	BodyRoot      [32]byte
}

type Checkpoint struct {
	Epoch uint64
	Root  [32]byte
}

type Deposit struct {
	Proof [33][32]byte
	Data  *DepositData
}

type DepositData struct {
	PublicKey             [48]byte
	WithdrawalCredentials [32]byte
	Amount                uint64
	Signature             [96]byte
}

type ETH1Data struct {
	DepositRoot  [32]byte
	DepositCount uint64
	BlockHash    [32]byte
}

type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader
	SignedHeader2 *SignedBeaconBlockHeader
}

type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature [96]byte
}

type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature [96]byte
}

type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

// Altair types

type AltairSyncAggregate struct {
	SyncCommitteeBits      [64]byte
	SyncCommitteeSignature [96]byte
}

// Bellatrix types

// Capella types

type CapellaBLSToExecutionChange struct {
	ValidatorIndex     uint64
	FromBLSPubkey      [48]byte
	ToExecutionAddress [20]byte
}

type CapellaSignedBLSToExecutionChange struct {
	Message   *CapellaBLSToExecutionChange
	Signature [96]byte
}

type CapellaWithdrawal struct {
	Index          uint64
	ValidatorIndex uint64
	Address        [20]byte
	Amount         uint64
}

// Deneb types

type DenebExecutionPayload struct {
	ParentHash    [32]byte
	FeeRecipient  [20]byte
	StateRoot     [32]byte
	ReceiptsRoot  [32]byte
	LogsBloom     [256]byte
	PrevRandao    [32]byte
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte `ssz-max:"32"`
	BaseFeePerGas *uint256.Int
	BlockHash     [32]byte
	Transactions  [][]byte             `ssz-max:"1048576,1073741824"`
	Withdrawals   []*CapellaWithdrawal `ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

// Electra types

type ElectraAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"131072"`
	Data            *AttestationData
	Signature       [96]byte
	CommitteeBits   [8]byte
}

type ElectraAttesterSlashing struct {
	Attestation1 *ElectraIndexedAttestation
	Attestation2 *ElectraIndexedAttestation
}

type ElectraBeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	Body          *ElectraBeaconBlockBody
}

type ElectraBeaconBlockBody struct {
	RANDAOReveal          [96]byte
	ETH1Data              *ETH1Data
	Graffiti              [32]byte
	ProposerSlashings     []*ProposerSlashing        `ssz-max:"16"`
	AttesterSlashings     []*ElectraAttesterSlashing `ssz-max:"1"`
	Attestations          []*ElectraAttestation      `ssz-max:"8"`
	Deposits              []*Deposit                 `ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit     `ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *DenebExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `ssz-max:"16"`
	BlobKZGCommitments    [][48]byte                           `ssz-max:"4096"`
	ExecutionRequests     *ElectraExecutionRequests
}

type ElectraConsolidationRequest struct {
	SourceAddress [20]byte
	SourcePubkey  [48]byte
	TargetPubkey  [48]byte
}

type ElectraDepositRequest struct {
	Pubkey                [48]byte
	WithdrawalCredentials [32]byte
	Amount                uint64
	Signature             [96]byte
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
	Signature        [96]byte
}

type ElectraSignedBeaconBlock struct {
	Message   *ElectraBeaconBlock
	Signature [96]byte
}

type ElectraWithdrawalRequest struct {
	SourceAddress   [20]byte
	ValidatorPubkey [48]byte
	Amount          uint64
}

// Fulu types
