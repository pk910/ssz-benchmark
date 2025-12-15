package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// Config holds the generator configuration
type Config struct {
	ValidatorCount         int
	TransactionCount       int
	TransactionMinSize     int
	TransactionMaxSize     int
	MaxAttestations        int
	MaxDeposits            int
	MaxProposerSlashings   int
	MaxAttesterSlashings   int
	MaxVoluntaryExits      int
	MaxBLSToExecChanges    int
	MaxWithdrawals         int
	MaxBlobCommitments     int
	SyncCommitteeSize      int
	SlotsPerHistoricalRoot int
	EpochsPerHistVector    int
	EpochsPerSlashVector   int
	SlotsPerEpoch          int
	EpochsPerEth1Voting    int
	Slot                   uint64
	OutputDir              string
	Seed                   int64
}

// PresetValues holds preset-specific values
type PresetValues struct {
	MaxWithdrawals         int
	MaxBlobCommitments     int
	SyncCommitteeSize      int
	SlotsPerHistoricalRoot int
	EpochsPerHistVector    int
	EpochsPerSlashVector   int
	SlotsPerEpoch          int
	EpochsPerEth1Voting    int
}

// Metadata for the generated files
type Metadata struct {
	HTR string `json:"htr"`
}

func main() {
	var cfg Config

	rootCmd := &cobra.Command{
		Use:   "generator",
		Short: "Generate random SSZ payloads for benchmarking",
		Long: `Generate random beacon blocks and states with configurable parameters
for SSZ benchmarking. Creates comparable payloads for both minimal and mainnet presets.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runGenerator(&cfg)
		},
	}

	rootCmd.Flags().IntVarP(&cfg.ValidatorCount, "validators", "v", 100000, "Number of validators")
	rootCmd.Flags().IntVarP(&cfg.TransactionCount, "transactions", "t", 100, "Number of transactions")
	rootCmd.Flags().IntVar(&cfg.TransactionMinSize, "tx-min-size", 500, "Minimum transaction size in bytes")
	rootCmd.Flags().IntVar(&cfg.TransactionMaxSize, "tx-max-size", 700, "Maximum transaction size in bytes")
	rootCmd.Flags().IntVar(&cfg.MaxAttestations, "attestations", 128, "Max attestations (up to 128)")
	rootCmd.Flags().IntVar(&cfg.MaxDeposits, "deposits", 16, "Max deposits (up to 16)")
	rootCmd.Flags().IntVar(&cfg.MaxProposerSlashings, "proposer-slashings", 16, "Max proposer slashings (up to 16)")
	rootCmd.Flags().IntVar(&cfg.MaxAttesterSlashings, "attester-slashings", 2, "Max attester slashings (up to 2)")
	rootCmd.Flags().IntVar(&cfg.MaxVoluntaryExits, "voluntary-exits", 16, "Max voluntary exits (up to 16)")
	rootCmd.Flags().IntVar(&cfg.MaxBLSToExecChanges, "bls-changes", 16, "Max BLS to execution changes (up to 16)")
	rootCmd.Flags().Uint64Var(&cfg.Slot, "slot", 1000, "Slot number for the generated block/state")
	rootCmd.Flags().StringVarP(&cfg.OutputDir, "output", "o", ".", "Output directory for generated files")
	rootCmd.Flags().Int64Var(&cfg.Seed, "seed", 0, "Random seed (0 for random)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGenerator(cfg *Config) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate for both presets
	presets := []struct {
		name   string
		file   string
		values PresetValues
	}{
		{
			name: "minimal",
			file: "minimal-preset.yaml",
			values: PresetValues{
				MaxWithdrawals:         4,
				MaxBlobCommitments:     32,
				SyncCommitteeSize:      32,
				SlotsPerHistoricalRoot: 64,
				EpochsPerHistVector:    64,
				EpochsPerSlashVector:   64,
				SlotsPerEpoch:          8,
				EpochsPerEth1Voting:    4,
			},
		},
		{
			name: "mainnet",
			file: "mainnet-preset.yaml",
			values: PresetValues{
				MaxWithdrawals:         16,
				MaxBlobCommitments:     4096,
				SyncCommitteeSize:      512,
				SlotsPerHistoricalRoot: 8192,
				EpochsPerHistVector:    65536,
				EpochsPerSlashVector:   8192,
				SlotsPerEpoch:          32,
				EpochsPerEth1Voting:    64,
			},
		},
	}

	for _, preset := range presets {
		fmt.Printf("Generating %s preset payloads...\n", preset.name)

		// Load preset
		specs, err := loadPreset(preset.file)
		if err != nil {
			return fmt.Errorf("failed to load %s preset: %w", preset.name, err)
		}

		// Create DynSsz instance
		dynSsz := dynssz.NewDynSsz(specs)

		// Generate block
		block := generateBlock(cfg, &preset.values)
		blockData, err := dynSsz.MarshalSSZ(block)
		if err != nil {
			return fmt.Errorf("failed to marshal %s block: %w", preset.name, err)
		}

		blockHtr, err := dynSsz.HashTreeRoot(block.Message)
		if err != nil {
			return fmt.Errorf("failed to compute %s block HTR: %w", preset.name, err)
		}

		blockPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("block-%s.ssz", preset.name))
		if err := os.WriteFile(blockPath, blockData, 0644); err != nil {
			return fmt.Errorf("failed to write %s block: %w", preset.name, err)
		}

		blockMetaPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("block-%s-meta.json", preset.name))
		if err := writeMetadata(blockMetaPath, blockHtr[:]); err != nil {
			return fmt.Errorf("failed to write %s block metadata: %w", preset.name, err)
		}

		fmt.Printf("  Block: %s (%d bytes, HTR: %s)\n", blockPath, len(blockData), hex.EncodeToString(blockHtr[:]))

		// Generate state
		state := generateState(cfg, &preset.values)
		stateData, err := dynSsz.MarshalSSZ(state)
		if err != nil {
			return fmt.Errorf("failed to marshal %s state: %w", preset.name, err)
		}

		stateHtr, err := dynSsz.HashTreeRoot(state)
		if err != nil {
			return fmt.Errorf("failed to compute %s state HTR: %w", preset.name, err)
		}

		statePath := filepath.Join(cfg.OutputDir, fmt.Sprintf("state-%s.ssz", preset.name))
		if err := os.WriteFile(statePath, stateData, 0644); err != nil {
			return fmt.Errorf("failed to write %s state: %w", preset.name, err)
		}

		stateMetaPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("state-%s-meta.json", preset.name))
		if err := writeMetadata(stateMetaPath, stateHtr[:]); err != nil {
			return fmt.Errorf("failed to write %s state metadata: %w", preset.name, err)
		}

		fmt.Printf("  State: %s (%d bytes, HTR: %s)\n", statePath, len(stateData), hex.EncodeToString(stateHtr[:]))
	}

	fmt.Println("Generation complete!")
	return nil
}

func loadPreset(filename string) (map[string]any, error) {
	// Try to find preset file relative to executable or current directory
	paths := []string{
		filename,
		filepath.Join("res/generator", filename),
	}

	// Also try relative to executable
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), filename))
	}

	var data []byte
	var err error
	for _, path := range paths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("preset file not found: %s", filename)
	}

	var specs map[string]any
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, fmt.Errorf("failed to parse preset: %w", err)
	}

	return specs, nil
}

func writeMetadata(path string, htr []byte) error {
	meta := Metadata{
		HTR: hex.EncodeToString(htr),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func generateBlock(cfg *Config, preset *PresetValues) *SignedBeaconBlock {
	return &SignedBeaconBlock{
		Message:   generateBeaconBlock(cfg, preset),
		Signature: randomBLSSignature(),
	}
}

func generateBeaconBlock(cfg *Config, preset *PresetValues) *BeaconBlock {
	return &BeaconBlock{
		Slot:          cfg.Slot,
		ProposerIndex: randomValidatorIndex(cfg.ValidatorCount),
		ParentRoot:    randomRoot(),
		StateRoot:     randomRoot(),
		Body:          generateBeaconBlockBody(cfg, preset),
	}
}

func generateBeaconBlockBody(cfg *Config, preset *PresetValues) *BeaconBlockBody {
	// Use minimum of configured value and preset max
	maxWithdrawals := min(preset.MaxWithdrawals, 16)
	maxBlobCommitments := min(preset.MaxBlobCommitments, 32) // cap for reasonable file size

	return &BeaconBlockBody{
		RANDAOReveal:          randomBLSSignature(),
		ETH1Data:              generateETH1Data(),
		Graffiti:              randomHash32(),
		ProposerSlashings:     generateProposerSlashings(min(cfg.MaxProposerSlashings, 16), cfg.ValidatorCount),
		AttesterSlashings:     generateAttesterSlashings(min(cfg.MaxAttesterSlashings, 2), cfg.ValidatorCount),
		Attestations:          generateAttestations(min(cfg.MaxAttestations, 128), cfg.ValidatorCount, cfg.Slot),
		Deposits:              generateDeposits(min(cfg.MaxDeposits, 16)),
		VoluntaryExits:        generateVoluntaryExits(min(cfg.MaxVoluntaryExits, 16), cfg.ValidatorCount),
		SyncAggregate:         generateSyncAggregate(preset.SyncCommitteeSize),
		ExecutionPayload:      generateExecutionPayload(cfg, maxWithdrawals),
		BLSToExecutionChanges: generateBLSToExecChanges(min(cfg.MaxBLSToExecChanges, 16), cfg.ValidatorCount),
		BlobKZGCommitments:    generateBlobCommitments(maxBlobCommitments),
	}
}

func generateState(cfg *Config, preset *PresetValues) *BeaconState {
	validators := make([]*Validator, cfg.ValidatorCount)
	balances := make([]Gwei, cfg.ValidatorCount)
	prevParticipation := make([]ParticipationFlags, cfg.ValidatorCount)
	currParticipation := make([]ParticipationFlags, cfg.ValidatorCount)
	inactivityScores := make([]uint64, cfg.ValidatorCount)

	for i := 0; i < cfg.ValidatorCount; i++ {
		validators[i] = generateValidator()
		balances[i] = 32000000000 + Gwei(randomUint64()%1000000000)
		prevParticipation[i] = ParticipationFlags(randomByte() & 0x07)
		currParticipation[i] = ParticipationFlags(randomByte() & 0x07)
		inactivityScores[i] = randomUint64() % 100
	}

	// Generate historical roots (block and state roots)
	blockRoots := make([]Root, preset.SlotsPerHistoricalRoot)
	stateRoots := make([]Root, preset.SlotsPerHistoricalRoot)
	for i := 0; i < preset.SlotsPerHistoricalRoot; i++ {
		blockRoots[i] = randomRoot()
		stateRoots[i] = randomRoot()
	}

	// Generate RANDAO mixes
	randaoMixes := make([]Root, preset.EpochsPerHistVector)
	for i := 0; i < preset.EpochsPerHistVector; i++ {
		randaoMixes[i] = randomRoot()
	}

	// Generate slashings
	slashings := make([]Gwei, preset.EpochsPerSlashVector)
	for i := 0; i < preset.EpochsPerSlashVector; i++ {
		slashings[i] = randomUint64() % 32000000000
	}

	// Generate ETH1 data votes
	maxEth1Votes := preset.SlotsPerEpoch * preset.EpochsPerEth1Voting
	eth1Votes := make([]*ETH1Data, maxEth1Votes)
	for i := 0; i < maxEth1Votes; i++ {
		eth1Votes[i] = generateETH1Data()
	}

	currentEpoch := cfg.Slot / uint64(preset.SlotsPerEpoch)

	return &BeaconState{
		GenesisTime:                  1606824023,
		GenesisValidatorsRoot:        randomRoot(),
		Slot:                         cfg.Slot,
		Fork:                         generateFork(currentEpoch),
		LatestBlockHeader:            generateBeaconBlockHeader(cfg.Slot, cfg.ValidatorCount),
		BlockRoots:                   blockRoots,
		StateRoots:                   stateRoots,
		HistoricalRoots:              []Root{}, // Empty for new chain
		ETH1Data:                     generateETH1Data(),
		ETH1DataVotes:                eth1Votes,
		ETH1DepositIndex:             uint64(cfg.ValidatorCount),
		Validators:                   validators,
		Balances:                     balances,
		RANDAOMixes:                  randaoMixes,
		Slashings:                    slashings,
		PreviousEpochParticipation:   prevParticipation,
		CurrentEpochParticipation:    currParticipation,
		JustificationBits:            bitfield.NewBitvector4(),
		PreviousJustifiedCheckpoint:  generateCheckpoint(currentEpoch - 2),
		CurrentJustifiedCheckpoint:   generateCheckpoint(currentEpoch - 1),
		FinalizedCheckpoint:          generateCheckpoint(currentEpoch - 2),
		InactivityScores:             inactivityScores,
		CurrentSyncCommittee:         generateSyncCommittee(preset.SyncCommitteeSize),
		NextSyncCommittee:            generateSyncCommittee(preset.SyncCommitteeSize),
		LatestExecutionPayloadHeader: generateExecutionPayloadHeader(),
		NextWithdrawalIndex:          randomUint64() % 1000000,
		NextWithdrawalValidatorIndex: randomValidatorIndex(cfg.ValidatorCount),
		HistoricalSummaries:          []*HistoricalSummary{}, // Empty for new chain
	}
}

// Helper functions for generating random data

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func randomByte() byte {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return b[0]
}

func randomUint64() uint64 {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func randomRoot() Root {
	var root Root
	_, _ = rand.Read(root[:])
	return root
}

func randomHash32() Hash32 {
	var hash Hash32
	_, _ = rand.Read(hash[:])
	return hash
}

func randomBLSPubKey() BLSPubKey {
	var key BLSPubKey
	_, _ = rand.Read(key[:])
	return key
}

func randomBLSSignature() BLSSignature {
	var sig BLSSignature
	_, _ = rand.Read(sig[:])
	return sig
}

func randomExecutionAddress() ExecutionAddress {
	var addr ExecutionAddress
	_, _ = rand.Read(addr[:])
	return addr
}

func randomKZGCommitment() KZGCommitment {
	var commitment KZGCommitment
	_, _ = rand.Read(commitment[:])
	return commitment
}

func randomValidatorIndex(maxValidators int) ValidatorIndex {
	return ValidatorIndex(randomUint64() % uint64(maxValidators))
}

func randomLogsBloom() LogsBloom {
	var bloom LogsBloom
	_, _ = rand.Read(bloom[:])
	return bloom
}

func randomUint256() Uint256 {
	var u Uint256
	_, _ = rand.Read(u[:])
	return u
}

func generateETH1Data() *ETH1Data {
	return &ETH1Data{
		DepositRoot:  randomRoot(),
		DepositCount: randomUint64() % 1000000,
		BlockHash:    randomHash32(),
	}
}

func generateValidator() *Validator {
	return &Validator{
		Pubkey:                     randomBLSPubKey(),
		WithdrawalCredentials:      randomHash32(),
		EffectiveBalance:           32000000000,
		Slashed:                    false,
		ActivationEligibilityEpoch: 0,
		ActivationEpoch:            0,
		ExitEpoch:                  ^uint64(0),
		WithdrawableEpoch:          ^uint64(0),
	}
}

func generateFork(currentEpoch uint64) *Fork {
	return &Fork{
		PreviousVersion: [4]byte{0x04, 0x00, 0x00, 0x00}, // Deneb
		CurrentVersion:  [4]byte{0x04, 0x00, 0x00, 0x00}, // Deneb
		Epoch:           currentEpoch,
	}
}

func generateCheckpoint(epoch uint64) *Checkpoint {
	return &Checkpoint{
		Epoch: epoch,
		Root:  randomRoot(),
	}
}

func generateBeaconBlockHeader(slot uint64, maxValidators int) *BeaconBlockHeader {
	return &BeaconBlockHeader{
		Slot:          slot - 1,
		ProposerIndex: randomValidatorIndex(maxValidators),
		ParentRoot:    randomRoot(),
		StateRoot:     randomRoot(),
		BodyRoot:      randomRoot(),
	}
}

func generateProposerSlashings(count int, maxValidators int) []*ProposerSlashing {
	slashings := make([]*ProposerSlashing, count)
	for i := 0; i < count; i++ {
		validatorIdx := randomValidatorIndex(maxValidators)
		slashings[i] = &ProposerSlashing{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          randomUint64() % 1000,
					ProposerIndex: validatorIdx,
					ParentRoot:    randomRoot(),
					StateRoot:     randomRoot(),
					BodyRoot:      randomRoot(),
				},
				Signature: randomBLSSignature(),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          randomUint64() % 1000,
					ProposerIndex: validatorIdx,
					ParentRoot:    randomRoot(),
					StateRoot:     randomRoot(),
					BodyRoot:      randomRoot(),
				},
				Signature: randomBLSSignature(),
			},
		}
	}
	return slashings
}

func generateAttesterSlashings(count int, maxValidators int) []*AttesterSlashing {
	slashings := make([]*AttesterSlashing, count)
	for i := 0; i < count; i++ {
		// Generate overlapping attesting indices for a valid attester slashing
		numIndices := 10 + int(randomUint64()%50)
		indices := make([]uint64, numIndices)
		for j := 0; j < numIndices; j++ {
			indices[j] = uint64(randomValidatorIndex(maxValidators))
		}

		slashings[i] = &AttesterSlashing{
			Attestation1: &IndexedAttestation{
				AttestingIndices: indices,
				Data:             generateAttestationData(randomUint64()%1000, maxValidators),
				Signature:        randomBLSSignature(),
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: indices,
				Data:             generateAttestationData(randomUint64()%1000, maxValidators),
				Signature:        randomBLSSignature(),
			},
		}
	}
	return slashings
}

func generateAttestationData(slot uint64, _ int) *AttestationData {
	epoch := slot / 32
	return &AttestationData{
		Slot:            slot,
		Index:           randomUint64() % 64,
		BeaconBlockRoot: randomRoot(),
		Source: &Checkpoint{
			Epoch: epoch - 1,
			Root:  randomRoot(),
		},
		Target: &Checkpoint{
			Epoch: epoch,
			Root:  randomRoot(),
		},
	}
}

func generateAttestations(count int, maxValidators int, slot uint64) []*Attestation {
	attestations := make([]*Attestation, count)
	for i := 0; i < count; i++ {
		// Generate aggregation bits with some validators participating
		numBits := 64 + int(randomUint64()%200)
		aggBits := bitfield.NewBitlist(uint64(numBits))
		// Set ~2/3 of bits
		for j := 0; j < numBits*2/3; j++ {
			aggBits.SetBitAt(uint64(j), true)
		}

		attestations[i] = &Attestation{
			AggregationBits: aggBits,
			Data:            generateAttestationData(slot-1, maxValidators),
			Signature:       randomBLSSignature(),
		}
	}
	return attestations
}

func generateDeposits(count int) []*Deposit {
	deposits := make([]*Deposit, count)
	for i := 0; i < count; i++ {
		// Generate merkle proof (33 x 32-byte hashes)
		proof := make([][]byte, 33)
		for j := 0; j < 33; j++ {
			proof[j] = randomBytes(32)
		}

		deposits[i] = &Deposit{
			Proof: proof,
			Data: &DepositData{
				Pubkey:                randomBLSPubKey(),
				WithdrawalCredentials: randomHash32(),
				Amount:                32000000000,
				Signature:             randomBLSSignature(),
			},
		}
	}
	return deposits
}

func generateVoluntaryExits(count int, maxValidators int) []*SignedVoluntaryExit {
	exits := make([]*SignedVoluntaryExit, count)
	for i := 0; i < count; i++ {
		exits[i] = &SignedVoluntaryExit{
			Message: &VoluntaryExit{
				Epoch:          randomUint64() % 1000,
				ValidatorIndex: randomValidatorIndex(maxValidators),
			},
			Signature: randomBLSSignature(),
		}
	}
	return exits
}

func generateSyncAggregate(syncCommitteeSize int) *SyncAggregate {
	// Create sync committee bits based on committee size
	var bits bitfield.Bitvector512
	if syncCommitteeSize <= 512 {
		// Set ~2/3 of bits to simulate participation
		for i := 0; i < syncCommitteeSize*2/3; i++ {
			bits.SetBitAt(uint64(i), true)
		}
	}

	return &SyncAggregate{
		SyncCommitteeBits:      bits,
		SyncCommitteeSignature: randomBLSSignature(),
	}
}

func generateSyncCommittee(size int) *SyncCommittee {
	pubkeys := make([]BLSPubKey, size)
	for i := 0; i < size; i++ {
		pubkeys[i] = randomBLSPubKey()
	}

	return &SyncCommittee{
		Pubkeys:         pubkeys,
		AggregatePubkey: randomBLSPubKey(),
	}
}

func generateExecutionPayload(cfg *Config, maxWithdrawals int) *ExecutionPayload {
	// Generate transactions
	transactions := make([][]byte, cfg.TransactionCount)
	for i := 0; i < cfg.TransactionCount; i++ {
		txSize := cfg.TransactionMinSize + int(randomUint64()%uint64(cfg.TransactionMaxSize-cfg.TransactionMinSize+1))
		transactions[i] = randomBytes(txSize)
	}

	// Generate withdrawals
	withdrawals := make([]*Withdrawal, maxWithdrawals)
	for i := 0; i < maxWithdrawals; i++ {
		withdrawals[i] = &Withdrawal{
			Index:          uint64(i),
			ValidatorIndex: randomValidatorIndex(cfg.ValidatorCount),
			Address:        randomExecutionAddress(),
			Amount:         randomUint64() % 32000000000,
		}
	}

	return &ExecutionPayload{
		ParentHash:    randomHash32(),
		FeeRecipient:  randomExecutionAddress(),
		StateRoot:     randomHash32(),
		ReceiptsRoot:  randomHash32(),
		LogsBloom:     randomLogsBloom(),
		PrevRandao:    randomHash32(),
		BlockNumber:   randomUint64() % 10000000,
		GasLimit:      30000000,
		GasUsed:       15000000 + randomUint64()%10000000,
		Timestamp:     1700000000 + randomUint64()%10000000,
		ExtraData:     randomBytes(32),
		BaseFeePerGas: randomUint256(),
		BlockHash:     randomHash32(),
		Transactions:  transactions,
		Withdrawals:   withdrawals,
		BlobGasUsed:   randomUint64() % 1000000,
		ExcessBlobGas: randomUint64() % 1000000,
	}
}

func generateExecutionPayloadHeader() *ExecutionPayloadHeader {
	// Compute a fake transactions root
	txRoot := sha256.Sum256(randomBytes(32))

	// Compute a fake withdrawals root
	wdRoot := sha256.Sum256(randomBytes(32))

	return &ExecutionPayloadHeader{
		ParentHash:       randomHash32(),
		FeeRecipient:     randomExecutionAddress(),
		StateRoot:        randomHash32(),
		ReceiptsRoot:     randomHash32(),
		LogsBloom:        randomLogsBloom(),
		PrevRandao:       randomHash32(),
		BlockNumber:      randomUint64() % 10000000,
		GasLimit:         30000000,
		GasUsed:          15000000 + randomUint64()%10000000,
		Timestamp:        1700000000 + randomUint64()%10000000,
		ExtraData:        randomBytes(32),
		BaseFeePerGas:    randomUint256(),
		BlockHash:        randomHash32(),
		TransactionsRoot: txRoot,
		WithdrawalsRoot:  wdRoot,
		BlobGasUsed:      randomUint64() % 1000000,
		ExcessBlobGas:    randomUint64() % 1000000,
	}
}

func generateBLSToExecChanges(count int, maxValidators int) []*SignedBLSToExecutionChange {
	changes := make([]*SignedBLSToExecutionChange, count)
	for i := 0; i < count; i++ {
		changes[i] = &SignedBLSToExecutionChange{
			Message: &BLSToExecutionChange{
				ValidatorIndex:     randomValidatorIndex(maxValidators),
				FromBLSPubkey:      randomBLSPubKey(),
				ToExecutionAddress: randomExecutionAddress(),
			},
			Signature: randomBLSSignature(),
		}
	}
	return changes
}

func generateBlobCommitments(count int) []KZGCommitment {
	commitments := make([]KZGCommitment, count)
	for i := 0; i < count; i++ {
		commitments[i] = randomKZGCommitment()
	}
	return commitments
}
