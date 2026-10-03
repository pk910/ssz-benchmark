package gloas

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

// Electra types

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

type ElectraWithdrawalRequest struct {
	SourceAddress   [20]byte
	ValidatorPubkey [48]byte
	Amount          uint64
}

// Fulu types

// Gloas types

type GloasAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"1099511627776"`
	Data            *AttestationData
	Signature       [96]byte
	CommitteeBits   [8]byte
}

type GloasAttesterSlashing struct {
	Attestation1 *GloasIndexedAttestation
	Attestation2 *GloasIndexedAttestation
}

type GloasBeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	Body          *GloasBeaconBlockBody
}

type GloasBeaconBlockBody struct {
	RANDAOReveal              [96]byte
	ETH1Data                  *ETH1Data
	Graffiti                  [32]byte
	ProposerSlashings         []*ProposerSlashing      `ssz-max:"1099511627776"`
	AttesterSlashings         []*GloasAttesterSlashing `ssz-max:"1099511627776"`
	Attestations              []*GloasAttestation      `ssz-max:"1099511627776"`
	Deposits                  []*Deposit               `ssz-max:"1099511627776"`
	VoluntaryExits            []*SignedVoluntaryExit   `ssz-max:"1099511627776"`
	SyncAggregate             *AltairSyncAggregate
	BLSToExecutionChanges     []*CapellaSignedBLSToExecutionChange `ssz-max:"1099511627776"`
	SignedExecutionPayloadBid *GloasSignedExecutionPayloadBid
	PayloadAttestations       []*GloasPayloadAttestation `ssz-max:"1099511627776"`
	ParentExecutionRequests   *GloasExecutionRequests
}

type GloasBuilderDepositRequest struct {
	Pubkey                [48]byte
	WithdrawalCredentials [32]byte
	Amount                uint64
	Signature             [96]byte
}

type GloasBuilderExitRequest struct {
	SourceAddress [20]byte
	Pubkey        [48]byte
}

type GloasExecutionPayload struct {
	ParentHash      [32]byte
	FeeRecipient    [20]byte
	StateRoot       [32]byte
	ReceiptsRoot    [32]byte
	LogsBloom       [256]byte
	PrevRandao      [32]byte
	BlockNumber     uint64
	GasLimit        uint64
	GasUsed         uint64
	Timestamp       uint64
	ExtraData       []byte `ssz-max:"32"`
	BaseFeePerGas   *uint256.Int
	BlockHash       [32]byte
	Transactions    [][]byte             `ssz-max:"1099511627776,1099511627776"`
	Withdrawals     []*CapellaWithdrawal `ssz-max:"1099511627776"`
	BlobGasUsed     uint64
	ExcessBlobGas   uint64
	BlockAccessList []byte `ssz-max:"1099511627776"`
	SlotNumber      uint64
}

type GloasExecutionPayloadBid struct {
	ParentBlockHash       [32]byte
	ParentBlockRoot       [32]byte
	BlockHash             [32]byte
	PrevRandao            [32]byte
	FeeRecipient          [20]byte
	GasLimit              uint64
	BuilderIndex          uint64
	Slot                  uint64
	Value                 uint64
	ExecutionPayment      uint64
	BlobKZGCommitments    [][48]byte `ssz-max:"1099511627776"`
	ExecutionRequestsRoot [32]byte
}

type GloasExecutionPayloadEnvelope struct {
	Payload               *GloasExecutionPayload
	ExecutionRequests     *GloasExecutionRequests
	BuilderIndex          uint64
	BeaconBlockRoot       [32]byte
	ParentBeaconBlockRoot [32]byte
}

type GloasExecutionRequests struct {
	Deposits        []*ElectraDepositRequest       `ssz-max:"1099511627776"`
	Withdrawals     []*ElectraWithdrawalRequest    `ssz-max:"1099511627776"`
	Consolidations  []*ElectraConsolidationRequest `ssz-max:"1099511627776"`
	BuilderDeposits []*GloasBuilderDepositRequest  `ssz-max:"1099511627776"`
	BuilderExits    []*GloasBuilderExitRequest     `ssz-max:"1099511627776"`
}

type GloasIndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"1099511627776"`
	Data             *AttestationData
	Signature        [96]byte
}

type GloasPayloadAttestation struct {
	AggregationBits [64]byte
	Data            *GloasPayloadAttestationData
	Signature       [96]byte
}

type GloasPayloadAttestationData struct {
	BeaconBlockRoot   [32]byte
	Slot              uint64
	PayloadPresent    bool
	BlobDataAvailable bool
}

type GloasSignedBeaconBlock struct {
	Message   *GloasBeaconBlock
	Signature [96]byte
}

type GloasSignedExecutionPayloadBid struct {
	Message   *GloasExecutionPayloadBid
	Signature [96]byte
}

type GloasSignedExecutionPayloadEnvelope struct {
	Message   *GloasExecutionPayloadEnvelope
	Signature [96]byte
}
