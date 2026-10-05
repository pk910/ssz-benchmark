// payload builds the benchmark payload: a real mainnet beacon state and a
// real block with every operation list filled to its limit, the real
// blocks of the following epoch as they are, and the same data converted
// to the Gloas types (progressive lists and containers) so both shapes
// hold identical content.
//
//	payload -state state-<slot>.ssz -block block-<slot>.ssz -blocks <dir> -spec spec.json -out <dir> [-minimal <consensus-specs dir>]
//
// Output: <out>/fulu/{spec.json,state.ssz,state.root,block.ssz,block.root,blocks/NNN.ssz,NNN.root}
// and <out>/gloas/{spec.json,state,block,blocks,envelope} plus meta.json.
// With -minimal, the same objects cut to the minimal preset (minimal.go)
// under <out>/fulu/minimal and <out>/gloas/minimal, each with its own
// spec.json, plus meta-minimal.json. The mainnet output does not depend on
// it: the minimal objects are derived after it is written, from the same
// extended objects.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/prysmaticlabs/go-bitfield"

	"realbench/types/gloas"
)

// Gloas preset values that are not part of the beacon API config response.
var gloasPreset = map[string]uint64{
	"PTC_SIZE":                 512,
	"MAX_PAYLOAD_ATTESTATIONS": 4,
	"MAX_BUILDER_DEPOSIT_REQUESTS_PER_PAYLOAD": 64,
	"MAX_BUILDER_EXIT_REQUESTS_PER_PAYLOAD":    16,
	"MAX_BUILDERS_PER_WITHDRAWALS_SWEEP":       16384,
}

func main() {
	var stateFile, blockFile, blocksDir, specFile, outDir, minimalDir string
	var pendingFill, minimalStep int
	flag.StringVar(&stateFile, "state", "", "real beacon state (Fulu)")
	flag.StringVar(&blockFile, "block", "", "real signed block (Electra/Fulu) to extend")
	flag.StringVar(&blocksDir, "blocks", "", "directory with real block-<slot>.ssz files to take as the block set")
	flag.StringVar(&specFile, "spec", "", "beacon API config/spec response")
	flag.StringVar(&outDir, "out", "", "output directory")
	flag.IntVar(&pendingFill, "pending", 65536, "entries in each pending list of the state")
	flag.StringVar(&minimalDir, "minimal", "", "consensus-specs checkout; writes the payload cut to its minimal preset as well")
	flag.IntVar(&minimalStep, "minimal-step", 8, "every n-th validator is kept in the minimal state")
	flag.Parse()
	if stateFile == "" || blockFile == "" || blocksDir == "" || specFile == "" || outDir == "" {
		flag.Usage()
		os.Exit(2)
	}

	specRaw, specs := loadSpecs(specFile)
	for k, v := range gloasPreset {
		if _, ok := specs[k]; !ok {
			specs[k] = v
			specRaw[k] = strconv.FormatUint(v, 10)
		}
	}
	ds := dynssz.NewDynSsz(specs)
	rng := rand.New(rand.NewPCG(1, 2))

	log.Printf("decoding state")
	state := new(gloas.FuluBeaconState)
	if err := ds.UnmarshalSSZ(state, must(os.ReadFile(stateFile))); err != nil {
		log.Fatalf("state: %v", err)
	}
	block := new(gloas.ElectraSignedBeaconBlock)
	if err := ds.UnmarshalSSZ(block, must(os.ReadFile(blockFile))); err != nil {
		log.Fatalf("block: %v", err)
	}
	meta := map[string]any{
		"source_state_slot": state.Slot,
		"source_block_slot": block.Message.Slot,
		"validators":        len(state.Validators),
		"real": map[string]int{
			"eth1_data_votes":             len(state.ETH1DataVotes),
			"pending_deposits":            len(state.PendingDeposits),
			"pending_partial_withdrawals": len(state.PendingPartialWithdrawals),
			"pending_consolidations":      len(state.PendingConsolidations),
			"attestations":                len(block.Message.Body.Attestations),
			"transactions":                len(block.Message.Body.ExecutionPayload.Transactions),
		},
	}

	active := activeValidators(state)
	extendState(state, specs, pendingFill, rng)
	extendBlock(block, state, active, specs, rng)

	files := must(filepath.Glob(filepath.Join(blocksDir, "block-*.ssz")))
	sort.Strings(files)
	var realBlocks []*gloas.ElectraSignedBeaconBlock
	for _, f := range files {
		b := new(gloas.ElectraSignedBeaconBlock)
		if err := ds.UnmarshalSSZ(b, must(os.ReadFile(f))); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		realBlocks = append(realBlocks, b)
	}

	// Fulu shape.
	fuluDir := filepath.Join(outDir, "fulu")
	must(0, os.MkdirAll(filepath.Join(fuluDir, "blocks"), 0o755))
	writeSpec(filepath.Join(fuluDir, "spec.json"), specRaw)
	log.Printf("encoding fulu state")
	write(ds, filepath.Join(fuluDir, "state"), state, state)
	write(ds, filepath.Join(fuluDir, "block"), block, block.Message)
	for i, b := range realBlocks {
		write(ds, filepath.Join(fuluDir, "blocks", fmt.Sprintf("%03d", i)), b, b.Message)
	}

	// Gloas shape: the same content in the Gloas types.
	gloasDir := filepath.Join(outDir, "gloas")
	must(0, os.MkdirAll(filepath.Join(gloasDir, "blocks"), 0o755))
	writeSpec(filepath.Join(gloasDir, "spec.json"), specRaw)
	log.Printf("converting to gloas")
	gstate := toGloasState(state, active, specs, rng)
	gblock, genv := toGloasBlock(ds, block, spec(specs, "MAX_PAYLOAD_ATTESTATIONS"), spec(specs, "PTC_SIZE"), rng)
	log.Printf("encoding gloas state")
	write(ds, filepath.Join(gloasDir, "state"), gstate, gstate)
	write(ds, filepath.Join(gloasDir, "block"), gblock, gblock.Message)
	write(ds, filepath.Join(gloasDir, "envelope"), genv, genv.Message)
	for i, b := range realBlocks {
		gb, _ := toGloasBlock(ds, b, 1, spec(specs, "PTC_SIZE"), rng)
		write(ds, filepath.Join(gloasDir, "blocks", fmt.Sprintf("%03d", i)), gb, gb.Message)
	}

	meta["blocks"] = len(files)
	meta["filled"] = map[string]int{
		"eth1_data_votes":             len(state.ETH1DataVotes),
		"pending_deposits":            len(state.PendingDeposits),
		"pending_partial_withdrawals": len(state.PendingPartialWithdrawals),
		"pending_consolidations":      len(state.PendingConsolidations),
		"proposer_slashings":          len(block.Message.Body.ProposerSlashings),
		"attester_slashings":          len(block.Message.Body.AttesterSlashings),
		"attester_slashing_indices":   len(block.Message.Body.AttesterSlashings[0].Attestation1.AttestingIndices),
		"attestations":                len(block.Message.Body.Attestations),
		"attestation_bits":            int(block.Message.Body.Attestations[0].AggregationBits.Len()),
		"deposits":                    len(block.Message.Body.Deposits),
		"voluntary_exits":             len(block.Message.Body.VoluntaryExits),
		"bls_to_execution_changes":    len(block.Message.Body.BLSToExecutionChanges),
		"blob_kzg_commitments":        len(block.Message.Body.BlobKZGCommitments),
		"deposit_requests":            len(block.Message.Body.ExecutionRequests.Deposits),
		"withdrawal_requests":         len(block.Message.Body.ExecutionRequests.Withdrawals),
		"consolidation_requests":      len(block.Message.Body.ExecutionRequests.Consolidations),
		"transactions":                len(block.Message.Body.ExecutionPayload.Transactions),
		"withdrawals":                 len(block.Message.Body.ExecutionPayload.Withdrawals),
		"gloas_payload_attestations":  len(gblock.Message.Body.PayloadAttestations),
		"gloas_ptc_window":            len(gstate.PTCWindow),
	}
	mj, _ := json.MarshalIndent(meta, "", "  ")
	must(0, os.WriteFile(filepath.Join(outDir, "meta.json"), mj, 0o644))
	fmt.Println(string(mj))

	if minimalDir != "" {
		writeMinimal(outDir, minimalDir, minimalStep, specRaw, state, block, realBlocks, rng)
	}
}

// writeMinimal derives the minimal-preset payload from the extended
// mainnet objects and writes it under <out>/<fork>/minimal.
func writeMinimal(outDir, specsDir string, step int, mainnetRaw map[string]any, state *gloas.FuluBeaconState, block *gloas.ElectraSignedBeaconBlock, realBlocks []*gloas.ElectraSignedBeaconBlock, rng *rand.Rand) {
	raw, specs, overrides := minimalSpecs(mainnetRaw, specsDir)
	ds := dynssz.NewDynSsz(specs)
	log.Printf("cutting to the minimal preset (every %d. validator)", step)
	mstate := toMinimalState(state, specs, step)
	mblock := toMinimalBlock(block, specs, step)
	mblocks := make([]*gloas.ElectraSignedBeaconBlock, 0, len(realBlocks))
	for _, b := range realBlocks {
		mblocks = append(mblocks, toMinimalBlock(b, specs, step))
	}

	fuluDir := filepath.Join(outDir, "fulu", "minimal")
	must(0, os.MkdirAll(filepath.Join(fuluDir, "blocks"), 0o755))
	writeSpec(filepath.Join(fuluDir, "spec.json"), raw)
	log.Printf("encoding minimal fulu state")
	write(ds, filepath.Join(fuluDir, "state"), mstate, mstate)
	write(ds, filepath.Join(fuluDir, "block"), mblock, mblock.Message)
	for i, b := range mblocks {
		write(ds, filepath.Join(fuluDir, "blocks", fmt.Sprintf("%03d", i)), b, b.Message)
	}

	gloasDir := filepath.Join(outDir, "gloas", "minimal")
	must(0, os.MkdirAll(filepath.Join(gloasDir, "blocks"), 0o755))
	writeSpec(filepath.Join(gloasDir, "spec.json"), raw)
	log.Printf("converting the minimal objects to gloas")
	active := activeValidators(mstate)
	gstate := toGloasState(mstate, active, specs, rng)
	gblock, genv := toGloasBlock(ds, mblock, spec(specs, "MAX_PAYLOAD_ATTESTATIONS"), spec(specs, "PTC_SIZE"), rng)
	write(ds, filepath.Join(gloasDir, "state"), gstate, gstate)
	write(ds, filepath.Join(gloasDir, "block"), gblock, gblock.Message)
	write(ds, filepath.Join(gloasDir, "envelope"), genv, genv.Message)
	for i, b := range mblocks {
		gb, _ := toGloasBlock(ds, b, 1, spec(specs, "PTC_SIZE"), rng)
		write(ds, filepath.Join(gloasDir, "blocks", fmt.Sprintf("%03d", i)), gb, gb.Message)
	}

	meta := map[string]any{
		"preset":         "minimal",
		"validator_step": step,
		"validators":     len(mstate.Validators),
		"spec_overrides": overrides,
		"counts": map[string]int{
			"block_roots":                 len(mstate.BlockRoots),
			"randao_mixes":                len(mstate.RANDAOMixes),
			"slashings":                   len(mstate.Slashings),
			"eth1_data_votes":             len(mstate.ETH1DataVotes),
			"sync_committee":              len(mstate.CurrentSyncCommittee.Pubkeys),
			"proposer_lookahead":          len(mstate.ProposerLookahead),
			"pending_deposits":            len(mstate.PendingDeposits),
			"pending_partial_withdrawals": len(mstate.PendingPartialWithdrawals),
			"pending_consolidations":      len(mstate.PendingConsolidations),
			"attestations":                len(mblock.Message.Body.Attestations),
			"attestation_bits":            int(mblock.Message.Body.Attestations[0].AggregationBits.Len()),
			"attester_slashing_indices":   len(mblock.Message.Body.AttesterSlashings[0].Attestation1.AttestingIndices),
			"withdrawals":                 len(mblock.Message.Body.ExecutionPayload.Withdrawals),
			"gloas_ptc_window":            len(gstate.PTCWindow),
			"gloas_ptc_size":              len(gstate.PTCWindow[0]),
			"blocks":                      len(mblocks),
		},
	}
	mj, _ := json.MarshalIndent(meta, "", "  ")
	must(0, os.WriteFile(filepath.Join(outDir, "meta-minimal.json"), mj, 0o644))
	fmt.Println(string(mj))
}

func must[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return v
}

// loadSpecs returns the raw config strings and the numeric spec map.
func loadSpecs(path string) (map[string]any, map[string]any) {
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(must(os.ReadFile(path)), &resp); err != nil {
		log.Fatal(err)
	}
	specs := make(map[string]any, len(resp.Data))
	for k, v := range resp.Data {
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				specs[k] = n
			}
		}
	}
	return resp.Data, specs
}

func writeSpec(path string, raw map[string]any) {
	data, err := json.Marshal(map[string]any{"data": raw})
	if err != nil {
		log.Fatal(err)
	}
	must(0, os.WriteFile(path, data, 0o644))
}

func spec(specs map[string]any, name string) int {
	v, ok := specs[name].(uint64)
	if !ok {
		log.Fatalf("spec %s missing", name)
	}
	return int(v)
}

// write encodes v and stores it with the hash tree root of hashTarget.
func write(ds *dynssz.DynSsz, base string, v any, hashTarget any) {
	data, err := ds.MarshalSSZ(v)
	if err != nil {
		log.Fatalf("%s: marshal: %v", base, err)
	}
	root, err := ds.HashTreeRoot(hashTarget)
	if err != nil {
		log.Fatalf("%s: root: %v", base, err)
	}
	must(0, os.WriteFile(base+".ssz", data, 0o644))
	must(0, os.WriteFile(base+".root", []byte("0x"+hex.EncodeToString(root[:])+"\n"), 0o644))
	log.Printf("%s: %d bytes, root %x", base, len(data), root[:8])
}

func randBytes(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.Uint32())
	}
	return b
}

func randSig(rng *rand.Rand) (s gloas.BLSSignature) {
	copy(s[:], randBytes(rng, 96))
	return s
}

func randKey(rng *rand.Rand) (k gloas.BLSPubKey) {
	copy(k[:], randBytes(rng, 48))
	return k
}

func randRoot(rng *rand.Rand) (r gloas.Root) {
	copy(r[:], randBytes(rng, 32))
	return r
}

func randAddr(rng *rand.Rand) (a gloas.ExecutionAddress) {
	copy(a[:], randBytes(rng, 20))
	return a
}

func activeValidators(st *gloas.FuluBeaconState) []uint64 {
	epoch := uint64(st.Slot) / 32
	active := make([]uint64, 0, 1<<20)
	for i, v := range st.Validators {
		if uint64(v.ActivationEpoch) <= epoch && uint64(v.ExitEpoch) > epoch {
			active = append(active, uint64(i))
		}
	}
	return active
}

// extendState fills the variable state lists: eth1 data votes to their
// maximum and the three pending queues to n entries each. New entries are
// seeded variations of real-shaped entries.
func extendState(st *gloas.FuluBeaconState, specs map[string]any, n int, rng *rand.Rand) {
	maxVotes := spec(specs, "EPOCHS_PER_ETH1_VOTING_PERIOD") * spec(specs, "SLOTS_PER_EPOCH")
	for i := len(st.ETH1DataVotes); len(st.ETH1DataVotes) < maxVotes; i++ {
		v := *st.ETH1Data
		if len(st.ETH1DataVotes) > 0 {
			v = *st.ETH1DataVotes[i%len(st.ETH1DataVotes)]
		}
		v.DepositCount += uint64(i)
		v.BlockHash = randBytes(rng, 32)
		v.DepositRoot = randRoot(rng)
		st.ETH1DataVotes = append(st.ETH1DataVotes, &v)
	}
	validators := uint64(len(st.Validators))
	for len(st.PendingDeposits) < n {
		d := &gloas.ElectraPendingDeposit{
			Pubkey:                randKey(rng),
			WithdrawalCredentials: randBytes(rng, 32),
			Amount:                gloas.Gwei(32_000_000_000 + rng.Uint64N(2016_000_000_000)),
			Signature:             randSig(rng),
			Slot:                  st.Slot - gloas.Slot(rng.Uint64N(64)),
		}
		d.WithdrawalCredentials[0] = 0x02
		st.PendingDeposits = append(st.PendingDeposits, d)
	}
	for len(st.PendingPartialWithdrawals) < n {
		st.PendingPartialWithdrawals = append(st.PendingPartialWithdrawals, &gloas.ElectraPendingPartialWithdrawal{
			ValidatorIndex:    gloas.ValidatorIndex(rng.Uint64N(validators)),
			Amount:            gloas.Gwei(1_000_000_000 + rng.Uint64N(31_000_000_000)),
			WithdrawableEpoch: gloas.Epoch(uint64(st.Slot)/32 + 1 + rng.Uint64N(256)),
		})
	}
	for len(st.PendingConsolidations) < n {
		st.PendingConsolidations = append(st.PendingConsolidations, &gloas.ElectraPendingConsolidation{
			SourceIndex: gloas.ValidatorIndex(rng.Uint64N(validators)),
			TargetIndex: gloas.ValidatorIndex(rng.Uint64N(validators)),
		})
	}
}

// extendBlock fills every operation list of the block body to the limit of
// the spec. The attestations and the attester slashing cover one whole slot
// of committees (every committee bit set, every participant attesting); the
// execution payload keeps its real transactions.
func extendBlock(sb *gloas.ElectraSignedBeaconBlock, st *gloas.FuluBeaconState, active []uint64, specs map[string]any, rng *rand.Rand) {
	body := sb.Message.Body
	epoch := uint64(st.Slot) / uint64(spec(specs, "SLOTS_PER_EPOCH"))
	perSlot := len(active) / spec(specs, "SLOTS_PER_EPOCH")
	committees := spec(specs, "MAX_COMMITTEES_PER_SLOT")

	// One slot's attesters, in index order as an indexed attestation holds them.
	slotIndices := make([]uint64, perSlot)
	start := rng.IntN(len(active) - perSlot)
	copy(slotIndices, active[start:start+perSlot])
	sort.Slice(slotIndices, func(i, j int) bool { return slotIndices[i] < slotIndices[j] })

	template := body.Attestations[0].Data
	attData := func(slot uint64) *gloas.AttestationData {
		d := *template
		d.Slot = gloas.Slot(slot)
		d.BeaconBlockRoot = randRoot(rng)
		d.Source = &gloas.Checkpoint{Epoch: template.Source.Epoch, Root: template.Source.Root}
		d.Target = &gloas.Checkpoint{Epoch: template.Target.Epoch, Root: template.Target.Root}
		return &d
	}

	header := func() *gloas.SignedBeaconBlockHeader {
		h := *st.LatestBlockHeader
		h.Slot = gloas.Slot(uint64(st.Slot) - rng.Uint64N(8192))
		h.ProposerIndex = gloas.ValidatorIndex(active[rng.IntN(len(active))])
		h.ParentRoot = randRoot(rng)
		h.StateRoot = randRoot(rng)
		h.BodyRoot = randRoot(rng)
		return &gloas.SignedBeaconBlockHeader{Message: &h, Signature: randSig(rng)}
	}
	body.ProposerSlashings = body.ProposerSlashings[:0]
	for len(body.ProposerSlashings) < spec(specs, "MAX_PROPOSER_SLASHINGS") {
		h1 := header()
		h2 := *h1.Message
		h2.BodyRoot = randRoot(rng)
		body.ProposerSlashings = append(body.ProposerSlashings, &gloas.ProposerSlashing{
			SignedHeader1: h1,
			SignedHeader2: &gloas.SignedBeaconBlockHeader{Message: &h2, Signature: randSig(rng)},
		})
	}

	body.AttesterSlashings = body.AttesterSlashings[:0]
	for len(body.AttesterSlashings) < spec(specs, "MAX_ATTESTER_SLASHINGS_ELECTRA") {
		body.AttesterSlashings = append(body.AttesterSlashings, &gloas.ElectraAttesterSlashing{
			Attestation1: &gloas.ElectraIndexedAttestation{AttestingIndices: append([]uint64(nil), slotIndices...), Data: attData(uint64(st.Slot) - 2), Signature: randSig(rng)},
			Attestation2: &gloas.ElectraIndexedAttestation{AttestingIndices: append([]uint64(nil), slotIndices...), Data: attData(uint64(st.Slot) - 2), Signature: randSig(rng)},
		})
	}

	body.Attestations = body.Attestations[:0]
	for len(body.Attestations) < spec(specs, "MAX_ATTESTATIONS_ELECTRA") {
		bits := bitfield.NewBitlist(uint64(perSlot))
		for i := range uint64(perSlot) {
			bits.SetBitAt(i, true)
		}
		cb := bitfield.NewBitvector64()
		for i := range uint64(committees) {
			cb.SetBitAt(i, true)
		}
		body.Attestations = append(body.Attestations, &gloas.ElectraAttestation{
			AggregationBits: bits,
			Data:            attData(uint64(sb.Message.Slot) - 1 - uint64(len(body.Attestations))),
			Signature:       randSig(rng),
			CommitteeBits:   cb,
		})
	}

	const depth = 33 // DEPOSIT_CONTRACT_TREE_DEPTH + 1, a constant of the spec, not in the config
	body.Deposits = body.Deposits[:0]
	for len(body.Deposits) < spec(specs, "MAX_DEPOSITS") {
		proof := make([][]byte, depth)
		for i := range proof {
			proof[i] = randBytes(rng, 32)
		}
		creds := randBytes(rng, 32)
		creds[0] = 0x01
		body.Deposits = append(body.Deposits, &gloas.Deposit{Proof: proof, Data: &gloas.DepositData{
			PublicKey: randKey(rng), WithdrawalCredentials: creds, Amount: 32_000_000_000, Signature: randSig(rng)}})
	}

	body.VoluntaryExits = body.VoluntaryExits[:0]
	for len(body.VoluntaryExits) < spec(specs, "MAX_VOLUNTARY_EXITS") {
		body.VoluntaryExits = append(body.VoluntaryExits, &gloas.SignedVoluntaryExit{
			Message:   &gloas.VoluntaryExit{Epoch: gloas.Epoch(epoch), ValidatorIndex: gloas.ValidatorIndex(active[rng.IntN(len(active))])},
			Signature: randSig(rng),
		})
	}

	body.BLSToExecutionChanges = body.BLSToExecutionChanges[:0]
	for len(body.BLSToExecutionChanges) < spec(specs, "MAX_BLS_TO_EXECUTION_CHANGES") {
		body.BLSToExecutionChanges = append(body.BLSToExecutionChanges, &gloas.CapellaSignedBLSToExecutionChange{
			Message:   &gloas.CapellaBLSToExecutionChange{ValidatorIndex: gloas.ValidatorIndex(active[rng.IntN(len(active))]), FromBLSPubkey: randKey(rng), ToExecutionAddress: randAddr(rng)},
			Signature: randSig(rng),
		})
	}

	body.BlobKZGCommitments = body.BlobKZGCommitments[:0]
	for len(body.BlobKZGCommitments) < maxBlobs(specs, epoch) {
		var c gloas.KZGCommitment
		copy(c[:], randBytes(rng, 48))
		c[0] = 0xc0
		body.BlobKZGCommitments = append(body.BlobKZGCommitments, c)
	}

	req := body.ExecutionRequests
	req.Deposits = req.Deposits[:0]
	for len(req.Deposits) < spec(specs, "MAX_DEPOSIT_REQUESTS_PER_PAYLOAD") {
		creds := randBytes(rng, 32)
		creds[0] = 0x02
		req.Deposits = append(req.Deposits, &gloas.ElectraDepositRequest{
			Pubkey: randKey(rng), WithdrawalCredentials: creds, Amount: 32_000_000_000, Signature: randSig(rng), Index: st.ETH1DepositIndex + uint64(len(req.Deposits))})
	}
	req.Withdrawals = req.Withdrawals[:0]
	for len(req.Withdrawals) < spec(specs, "MAX_WITHDRAWAL_REQUESTS_PER_PAYLOAD") {
		req.Withdrawals = append(req.Withdrawals, &gloas.ElectraWithdrawalRequest{SourceAddress: randAddr(rng), ValidatorPubkey: randKey(rng), Amount: gloas.Gwei(rng.Uint64N(32_000_000_000))})
	}
	req.Consolidations = req.Consolidations[:0]
	for len(req.Consolidations) < spec(specs, "MAX_CONSOLIDATION_REQUESTS_PER_PAYLOAD") {
		req.Consolidations = append(req.Consolidations, &gloas.ElectraConsolidationRequest{SourceAddress: randAddr(rng), SourcePubkey: randKey(rng), TargetPubkey: randKey(rng)})
	}

	payload := body.ExecutionPayload
	for len(payload.Withdrawals) < spec(specs, "MAX_WITHDRAWALS_PER_PAYLOAD") {
		w := *payload.Withdrawals[len(payload.Withdrawals)-1]
		w.Index++
		payload.Withdrawals = append(payload.Withdrawals, &w)
	}
	sb.Signature = randSig(rng)
}

// maxBlobs is the blob limit in force at the epoch: the last BLOB_SCHEDULE
// entry at or before it, else MAX_BLOBS_PER_BLOCK_ELECTRA.
func maxBlobs(specs map[string]any, epoch uint64) int {
	n := spec(specs, "MAX_BLOBS_PER_BLOCK_ELECTRA")
	if sched, ok := specs["BLOB_SCHEDULE"].([]any); ok {
		for _, e := range sched {
			m, _ := e.(map[string]any)
			ep, _ := strconv.ParseUint(fmt.Sprint(m["EPOCH"]), 10, 64)
			mb, _ := strconv.Atoi(fmt.Sprint(m["MAX_BLOBS_PER_BLOCK"]))
			if ep <= epoch && mb > 0 {
				n = mb
			}
		}
	}
	return n
}

// toGloasState carries every field of the Fulu state over to the Gloas
// state (the shared element types are the same Go types, so the lists are
// shared, not copied) and fills the fields Gloas adds: the latest payload
// bid from the latest payload header, full payload availability, an empty
// builder registry and the PTC window with real validator indices.
func toGloasState(st *gloas.FuluBeaconState, active []uint64, specs map[string]any, rng *rand.Rand) *gloas.GloasBeaconState {
	h := st.LatestExecutionPayloadHeader
	g := &gloas.GloasBeaconState{
		GenesisTime:                   st.GenesisTime,
		GenesisValidatorsRoot:         st.GenesisValidatorsRoot,
		Slot:                          st.Slot,
		Fork:                          st.Fork,
		LatestBlockHeader:             st.LatestBlockHeader,
		BlockRoots:                    st.BlockRoots,
		StateRoots:                    st.StateRoots,
		HistoricalRoots:               st.HistoricalRoots,
		ETH1Data:                      st.ETH1Data,
		ETH1DataVotes:                 st.ETH1DataVotes,
		ETH1DepositIndex:              st.ETH1DepositIndex,
		Validators:                    st.Validators,
		Balances:                      st.Balances,
		RANDAOMixes:                   st.RANDAOMixes,
		Slashings:                     st.Slashings,
		PreviousEpochParticipation:    st.PreviousEpochParticipation,
		CurrentEpochParticipation:     st.CurrentEpochParticipation,
		JustificationBits:             st.JustificationBits,
		PreviousJustifiedCheckpoint:   st.PreviousJustifiedCheckpoint,
		CurrentJustifiedCheckpoint:    st.CurrentJustifiedCheckpoint,
		FinalizedCheckpoint:           st.FinalizedCheckpoint,
		InactivityScores:              st.InactivityScores,
		CurrentSyncCommittee:          st.CurrentSyncCommittee,
		NextSyncCommittee:             st.NextSyncCommittee,
		LatestBlockHash:               h.BlockHash,
		NextWithdrawalIndex:           st.NextWithdrawalIndex,
		NextWithdrawalValidatorIndex:  st.NextWithdrawalValidatorIndex,
		HistoricalSummaries:           st.HistoricalSummaries,
		DepositRequestsStartIndex:     st.DepositRequestsStartIndex,
		DepositBalanceToConsume:       st.DepositBalanceToConsume,
		ExitBalanceToConsume:          st.ExitBalanceToConsume,
		EarliestExitEpoch:             st.EarliestExitEpoch,
		ConsolidationBalanceToConsume: st.ConsolidationBalanceToConsume,
		EarliestConsolidationEpoch:    st.EarliestConsolidationEpoch,
		PendingDeposits:               st.PendingDeposits,
		PendingPartialWithdrawals:     st.PendingPartialWithdrawals,
		PendingConsolidations:         st.PendingConsolidations,
		ProposerLookahead:             st.ProposerLookahead,
		Builders:                      []*gloas.GloasBuilder{},
		ExecutionPayloadAvailability:  make([]uint8, spec(specs, "SLOTS_PER_HISTORICAL_ROOT")/8),
		BuilderPendingWithdrawals:     []*gloas.GloasBuilderPendingWithdrawal{},
		PayloadExpectedWithdrawals:    []*gloas.CapellaWithdrawal{},
		LatestExecutionPayloadBid: &gloas.GloasExecutionPayloadBid{
			ParentBlockHash:    h.ParentHash,
			ParentBlockRoot:    st.LatestBlockHeader.ParentRoot,
			BlockHash:          h.BlockHash,
			PrevRandao:         h.PrevRandao,
			FeeRecipient:       h.FeeRecipient,
			GasLimit:           h.GasLimit,
			Slot:               st.Slot,
			BlobKZGCommitments: []gloas.KZGCommitment{},
		},
	}
	for i := range g.ExecutionPayloadAvailability {
		g.ExecutionPayloadAvailability[i] = 0xff
	}
	for range spec(specs, "SLOTS_PER_EPOCH") * 2 {
		g.BuilderPendingPayments = append(g.BuilderPendingPayments, &gloas.GloasBuilderPendingPayment{Withdrawal: &gloas.GloasBuilderPendingWithdrawal{}})
	}
	windowSlots := (2 + spec(specs, "MIN_SEED_LOOKAHEAD")) * spec(specs, "SLOTS_PER_EPOCH")
	ptc := spec(specs, "PTC_SIZE")
	g.PTCWindow = make([][]gloas.ValidatorIndex, windowSlots)
	for i := range g.PTCWindow {
		g.PTCWindow[i] = make([]gloas.ValidatorIndex, ptc)
		for k := range g.PTCWindow[i] {
			g.PTCWindow[i][k] = gloas.ValidatorIndex(active[rng.IntN(len(active))])
		}
	}
	return g
}

// toGloasBlock carries a block over to the Gloas types: the consensus
// operations as they are, the execution payload replaced by a bid built
// from it, payload attestations added, and the execution requests moved to
// the parent requests. The payload itself goes into the envelope.
func toGloasBlock(ds *dynssz.DynSsz, sb *gloas.ElectraSignedBeaconBlock, payloadAtts, ptc int, rng *rand.Rand) (*gloas.GloasSignedBeaconBlock, *gloas.GloasSignedExecutionPayloadEnvelope) {
	b := sb.Message.Body
	p := b.ExecutionPayload
	req := &gloas.GloasExecutionRequests{
		Deposits:        b.ExecutionRequests.Deposits,
		Withdrawals:     b.ExecutionRequests.Withdrawals,
		Consolidations:  b.ExecutionRequests.Consolidations,
		BuilderDeposits: []*gloas.GloasBuilderDepositRequest{},
		BuilderExits:    []*gloas.GloasBuilderExitRequest{},
	}
	reqRoot, err := ds.HashTreeRoot(req)
	if err != nil {
		log.Fatalf("requests root: %v", err)
	}
	var atts []*gloas.GloasAttestation
	for _, a := range b.Attestations {
		atts = append(atts, &gloas.GloasAttestation{AggregationBits: a.AggregationBits, Data: a.Data, Signature: a.Signature, CommitteeBits: a.CommitteeBits})
	}
	var slashings []*gloas.GloasAttesterSlashing
	for _, s := range b.AttesterSlashings {
		slashings = append(slashings, &gloas.GloasAttesterSlashing{
			Attestation1: &gloas.GloasIndexedAttestation{AttestingIndices: s.Attestation1.AttestingIndices, Data: s.Attestation1.Data, Signature: s.Attestation1.Signature},
			Attestation2: &gloas.GloasIndexedAttestation{AttestingIndices: s.Attestation2.AttestingIndices, Data: s.Attestation2.Data, Signature: s.Attestation2.Signature},
		})
	}
	var payloadAttestations []*gloas.GloasPayloadAttestation
	for range payloadAtts {
		bits := make(bitfield.Bitvector512, (ptc+7)/8)
		for i := range ptc {
			bits[i/8] |= 1 << (i % 8)
		}
		payloadAttestations = append(payloadAttestations, &gloas.GloasPayloadAttestation{
			AggregationBits: bits,
			Data:            &gloas.GloasPayloadAttestationData{BeaconBlockRoot: sb.Message.ParentRoot, Slot: sb.Message.Slot - 1, PayloadPresent: true, BlobDataAvailable: true},
			Signature:       randSig(rng),
		})
	}
	block := &gloas.GloasSignedBeaconBlock{
		Message: &gloas.GloasBeaconBlock{
			Slot:          sb.Message.Slot,
			ProposerIndex: sb.Message.ProposerIndex,
			ParentRoot:    sb.Message.ParentRoot,
			StateRoot:     sb.Message.StateRoot,
			Body: &gloas.GloasBeaconBlockBody{
				RANDAOReveal:          b.RANDAOReveal,
				ETH1Data:              b.ETH1Data,
				Graffiti:              b.Graffiti,
				ProposerSlashings:     b.ProposerSlashings,
				AttesterSlashings:     slashings,
				Attestations:          atts,
				Deposits:              b.Deposits,
				VoluntaryExits:        b.VoluntaryExits,
				SyncAggregate:         b.SyncAggregate,
				BLSToExecutionChanges: b.BLSToExecutionChanges,
				SignedExecutionPayloadBid: &gloas.GloasSignedExecutionPayloadBid{
					Message: &gloas.GloasExecutionPayloadBid{
						ParentBlockHash:       p.ParentHash,
						ParentBlockRoot:       sb.Message.ParentRoot,
						BlockHash:             p.BlockHash,
						PrevRandao:            p.PrevRandao,
						FeeRecipient:          p.FeeRecipient,
						GasLimit:              p.GasLimit,
						Slot:                  sb.Message.Slot,
						Value:                 gloas.Gwei(rng.Uint64N(1_000_000_000)),
						ExecutionPayment:      gloas.Gwei(rng.Uint64N(100_000_000)),
						BlobKZGCommitments:    b.BlobKZGCommitments,
						ExecutionRequestsRoot: reqRoot,
					},
					Signature: randSig(rng),
				},
				PayloadAttestations:     payloadAttestations,
				ParentExecutionRequests: req,
			},
		},
		Signature: sb.Signature,
	}
	if block.Message.Body.SignedExecutionPayloadBid.Message.BlobKZGCommitments == nil {
		block.Message.Body.SignedExecutionPayloadBid.Message.BlobKZGCommitments = []gloas.KZGCommitment{}
	}
	blockRoot, err := ds.HashTreeRoot(block.Message)
	if err != nil {
		log.Fatalf("gloas block root: %v", err)
	}
	envelope := &gloas.GloasSignedExecutionPayloadEnvelope{
		Message: &gloas.GloasExecutionPayloadEnvelope{
			Payload: &gloas.GloasExecutionPayload{
				ParentHash:      p.ParentHash,
				FeeRecipient:    p.FeeRecipient,
				StateRoot:       p.StateRoot,
				ReceiptsRoot:    p.ReceiptsRoot,
				LogsBloom:       p.LogsBloom,
				PrevRandao:      p.PrevRandao,
				BlockNumber:     p.BlockNumber,
				GasLimit:        p.GasLimit,
				GasUsed:         p.GasUsed,
				Timestamp:       p.Timestamp,
				ExtraData:       p.ExtraData,
				BaseFeePerGas:   p.BaseFeePerGas,
				BlockHash:       p.BlockHash,
				Transactions:    p.Transactions,
				Withdrawals:     p.Withdrawals,
				BlobGasUsed:     p.BlobGasUsed,
				ExcessBlobGas:   p.ExcessBlobGas,
				BlockAccessList: gloas.BlockAccessList{},
				SlotNumber:      uint64(sb.Message.Slot),
			},
			ExecutionRequests:     req,
			BeaconBlockRoot:       blockRoot,
			ParentBeaconBlockRoot: sb.Message.ParentRoot,
		},
		Signature: randSig(rng),
	}
	return block, envelope
}
