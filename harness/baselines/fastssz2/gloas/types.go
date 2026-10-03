package gloas

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

type VoluntaryExit struct {
	Epoch          uint64
	ValidatorIndex uint64
}

// Altair types

type AltairSyncAggregate struct {
	SyncCommitteeBits      [64]byte `ssz-size:"64"`
	SyncCommitteeSignature [96]byte `ssz-size:"96"`
}

// Bellatrix types

// Capella types

type CapellaBLSToExecutionChange struct {
	ValidatorIndex     uint64
	FromBLSPubkey      [48]byte `ssz-size:"48"`
	ToExecutionAddress [20]byte `ssz-size:"20"`
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

// Electra types

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

type ElectraWithdrawalRequest struct {
	SourceAddress   [20]byte `ssz-size:"20"`
	ValidatorPubkey [48]byte `ssz-size:"48"`
	Amount          uint64
}

// Fulu types

// Gloas types

type GloasAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"1099511627776" ssz:"bitlist"`
	Data            *AttestationData
	Signature       [96]byte `ssz-size:"96"`
	CommitteeBits   [8]byte  `ssz-size:"8"`
}

type GloasAttesterSlashing struct {
	Attestation1 *GloasIndexedAttestation
	Attestation2 *GloasIndexedAttestation
}

type GloasBeaconBlock struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte `ssz-size:"32"`
	StateRoot     [32]byte `ssz-size:"32"`
	Body          *GloasBeaconBlockBody
}

type GloasBeaconBlockBody struct {
	RANDAOReveal              [96]byte `ssz-size:"96"`
	ETH1Data                  *ETH1Data
	Graffiti                  [32]byte                 `ssz-size:"32"`
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
	Pubkey                [48]byte `ssz-size:"48"`
	WithdrawalCredentials []byte   `ssz-size:"32"`
	Amount                uint64
	Signature             [96]byte `ssz-size:"96"`
}

type GloasBuilderExitRequest struct {
	SourceAddress [20]byte `ssz-size:"20"`
	Pubkey        [48]byte `ssz-size:"48"`
}

type GloasExecutionPayload struct {
	ParentHash      [32]byte  `ssz-size:"32"`
	FeeRecipient    [20]byte  `ssz-size:"20"`
	StateRoot       [32]byte  `ssz-size:"32"`
	ReceiptsRoot    [32]byte  `ssz-size:"32"`
	LogsBloom       [256]byte `ssz-size:"256"`
	PrevRandao      [32]byte  `ssz-size:"32"`
	BlockNumber     uint64
	GasLimit        uint64
	GasUsed         uint64
	Timestamp       uint64
	ExtraData       []byte               `ssz-max:"32"`
	BaseFeePerGas   [32]byte             `ssz-size:"32"`
	BlockHash       [32]byte             `ssz-size:"32"`
	Transactions    [][]byte             `ssz-max:"1099511627776,1099511627776" ssz-size:"?,?"`
	Withdrawals     []*CapellaWithdrawal `ssz-max:"1099511627776"`
	BlobGasUsed     uint64
	ExcessBlobGas   uint64
	BlockAccessList []byte `ssz-max:"1099511627776"`
	SlotNumber      uint64
}

type GloasExecutionPayloadBid struct {
	ParentBlockHash       [32]byte `ssz-size:"32"`
	ParentBlockRoot       [32]byte `ssz-size:"32"`
	BlockHash             [32]byte `ssz-size:"32"`
	PrevRandao            [32]byte `ssz-size:"32"`
	FeeRecipient          [20]byte `ssz-size:"20"`
	GasLimit              uint64
	BuilderIndex          uint64
	Slot                  uint64
	Value                 uint64
	ExecutionPayment      uint64
	BlobKZGCommitments    [][48]byte `ssz-max:"1099511627776" ssz-size:"?,48"`
	ExecutionRequestsRoot [32]byte   `ssz-size:"32"`
}

type GloasExecutionPayloadEnvelope struct {
	Payload               *GloasExecutionPayload
	ExecutionRequests     *GloasExecutionRequests
	BuilderIndex          uint64
	BeaconBlockRoot       [32]byte `ssz-size:"32"`
	ParentBeaconBlockRoot [32]byte `ssz-size:"32"`
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
	Signature        [96]byte `ssz-size:"96"`
}

type GloasPayloadAttestation struct {
	AggregationBits [64]byte `ssz-size:"64"`
	Data            *GloasPayloadAttestationData
	Signature       [96]byte `ssz-size:"96"`
}

type GloasPayloadAttestationData struct {
	BeaconBlockRoot   [32]byte `ssz-size:"32"`
	Slot              uint64
	PayloadPresent    bool
	BlobDataAvailable bool
}

type GloasSignedBeaconBlock struct {
	Message   *GloasBeaconBlock
	Signature [96]byte `ssz-size:"96"`
}

type GloasSignedExecutionPayloadBid struct {
	Message   *GloasExecutionPayloadBid
	Signature [96]byte `ssz-size:"96"`
}

type GloasSignedExecutionPayloadEnvelope struct {
	Message   *GloasExecutionPayloadEnvelope
	Signature [96]byte `ssz-size:"96"`
}
