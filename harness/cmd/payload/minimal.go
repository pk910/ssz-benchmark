package main

import (
	"bufio"
	"log"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/prysmaticlabs/go-bitfield"

	"realbench/types/gloas"
)

// The minimal preset: the same payload cut to what the minimal preset's
// vectors and limits can hold. The validator registry is subsampled (every
// step-th validator), since the preset stands for small networks and the
// registry is what the cost of a state operation scales with; everything
// else is kept where the preset allows it and cut to the first entries,
// or the newest slots, where it does not.

// specLine matches one scalar entry of a consensus-specs yaml file.
var specLine = regexp.MustCompile(`^([A-Z][A-Z0-9_]*):\s*([^#\s]+)`)

// minimalSpecs derives the minimal spec response from the mainnet one:
// every decimal value of the consensus-specs minimal preset and config
// (<specsDir>/presets/minimal/*.yaml, <specsDir>/configs/minimal.yaml)
// replaces its mainnet value, an empty list replaces a schedule, and the
// preset names change. Returns the raw response (for spec.json), the
// numeric map and the values that differ from mainnet.
func minimalSpecs(mainnetRaw map[string]any, specsDir string) (map[string]any, map[string]any, map[string]string) {
	files := must(filepath.Glob(filepath.Join(specsDir, "presets", "minimal", "*.yaml")))
	sort.Strings(files)
	files = append(files, filepath.Join(specsDir, "configs", "minimal.yaml"))
	raw := make(map[string]any, len(mainnetRaw))
	maps.Copy(raw, mainnetRaw)
	overrides := make(map[string]string, 32)
	for _, f := range files {
		fh := must(os.Open(f))
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			m := specLine.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			key, val := m[1], m[2]
			if val == "[]" {
				if _, ok := raw[key].([]any); ok {
					raw[key] = []any{}
					overrides[key] = "[]"
				}
				continue
			}
			if _, err := strconv.ParseUint(val, 10, 64); err != nil {
				continue
			}
			if old, ok := raw[key].(string); !ok || old != val {
				overrides[key] = val
			}
			raw[key] = val
		}
		if err := sc.Err(); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		fh.Close()
	}
	if len(overrides) == 0 {
		log.Fatalf("no minimal preset values found under %s", specsDir)
	}
	raw["PRESET_BASE"], raw["CONFIG_NAME"] = "minimal", "minimal"
	specs := make(map[string]any, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				specs[k] = n
			}
		}
	}
	return raw, specs, overrides
}

// head is the first n entries of xs, or all of them.
func head[T any](xs []T, n int) []T {
	if len(xs) > n {
		xs = xs[:n]
	}
	return append(make([]T, 0, len(xs)), xs...)
}

// window cuts a vector indexed by slot or epoch modulo its length to n
// entries indexed modulo n: the entries of the n positions up to cur keep
// their places.
func window[T any](old []T, n int, cur uint64) []T {
	out := make([]T, n)
	for k := uint64(0); k < uint64(n); k++ {
		pos := cur - uint64(n-1) + k
		out[pos%uint64(n)] = old[pos%uint64(len(old))]
	}
	return out
}

// every takes every step-th entry of xs.
func every[T any](xs []T, step int) []T {
	out := make([]T, 0, (len(xs)+step-1)/step)
	for i := 0; i < len(xs); i += step {
		out = append(out, xs[i])
	}
	return out
}

// toMinimalState cuts the Fulu state to the minimal preset: every step-th
// validator with its balance, participation and inactivity score; the
// vectors cut to their minimal length keeping the newest slots and epochs
// in their places; the lists cut to their minimal limit keeping the first
// entries (the real ones come first); the sync committees' first pubkeys.
// Validator indices the state refers to are divided by step so they stay
// inside the registry. Pending deposits keep their limit and stay as they
// are. The input is not modified: what changes is copied.
func toMinimalState(st *gloas.FuluBeaconState, specs map[string]any, step int) *gloas.FuluBeaconState {
	slot := uint64(st.Slot)
	epoch := slot / 32 // the mainnet epoch, where the real entries are
	index := func(i gloas.ValidatorIndex) gloas.ValidatorIndex { return i / gloas.ValidatorIndex(step) }
	syncCommittee := func(c *gloas.AltairSyncCommittee) *gloas.AltairSyncCommittee {
		return &gloas.AltairSyncCommittee{Pubkeys: head(c.Pubkeys, spec(specs, "SYNC_COMMITTEE_SIZE")), AggregatePubkey: c.AggregatePubkey}
	}
	header := *st.LatestBlockHeader
	header.ProposerIndex = index(header.ProposerIndex)
	out := *st
	out.LatestBlockHeader = &header
	out.BlockRoots = window(st.BlockRoots, spec(specs, "SLOTS_PER_HISTORICAL_ROOT"), slot)
	out.StateRoots = window(st.StateRoots, spec(specs, "SLOTS_PER_HISTORICAL_ROOT"), slot)
	out.ETH1DataVotes = head(st.ETH1DataVotes, spec(specs, "EPOCHS_PER_ETH1_VOTING_PERIOD")*spec(specs, "SLOTS_PER_EPOCH"))
	out.Validators = every(st.Validators, step)
	out.Balances = every(st.Balances, step)
	out.RANDAOMixes = window(st.RANDAOMixes, spec(specs, "EPOCHS_PER_HISTORICAL_VECTOR"), epoch)
	out.Slashings = window(st.Slashings, spec(specs, "EPOCHS_PER_SLASHINGS_VECTOR"), epoch)
	out.PreviousEpochParticipation = every(st.PreviousEpochParticipation, step)
	out.CurrentEpochParticipation = every(st.CurrentEpochParticipation, step)
	out.InactivityScores = every(st.InactivityScores, step)
	out.CurrentSyncCommittee = syncCommittee(st.CurrentSyncCommittee)
	out.NextSyncCommittee = syncCommittee(st.NextSyncCommittee)
	out.NextWithdrawalValidatorIndex = index(st.NextWithdrawalValidatorIndex)
	out.PendingDeposits = head(st.PendingDeposits, spec(specs, "PENDING_DEPOSITS_LIMIT"))
	out.PendingPartialWithdrawals = make([]*gloas.ElectraPendingPartialWithdrawal, 0, spec(specs, "PENDING_PARTIAL_WITHDRAWALS_LIMIT"))
	for _, w := range head(st.PendingPartialWithdrawals, spec(specs, "PENDING_PARTIAL_WITHDRAWALS_LIMIT")) {
		c := *w
		c.ValidatorIndex = index(c.ValidatorIndex)
		out.PendingPartialWithdrawals = append(out.PendingPartialWithdrawals, &c)
	}
	out.PendingConsolidations = make([]*gloas.ElectraPendingConsolidation, 0, spec(specs, "PENDING_CONSOLIDATIONS_LIMIT"))
	for _, p := range head(st.PendingConsolidations, spec(specs, "PENDING_CONSOLIDATIONS_LIMIT")) {
		out.PendingConsolidations = append(out.PendingConsolidations, &gloas.ElectraPendingConsolidation{SourceIndex: index(p.SourceIndex), TargetIndex: index(p.TargetIndex)})
	}
	out.ProposerLookahead = head(st.ProposerLookahead, (spec(specs, "MIN_SEED_LOOKAHEAD")+1)*spec(specs, "SLOTS_PER_EPOCH"))
	for i := range out.ProposerLookahead {
		out.ProposerLookahead[i] = index(out.ProposerLookahead[i])
	}
	return &out
}

// bitvector is a bitvector of n bits as its bytes, with the bits of old
// below n kept.
func bitvector(old []byte, n int) []byte {
	out := make([]byte, (n+7)/8)
	for i := range n {
		if i/8 < len(old) && old[i/8]&(1<<(i%8)) != 0 {
			out[i/8] |= 1 << (i % 8)
		}
	}
	return out
}

// toMinimalBlock cuts a block to the minimal preset: the aggregation bits
// and the attesting indices of a slashing to the limit of a slot's
// committees (the preset cannot hold a mainnet slot's attesters), the
// committee bits and the sync committee bits to the committee sizes, the
// withdrawals to the payload limit. Single validator indices are divided by
// step as in the state; the attesting indices are the first entries as
// they are. The input is not modified.
func toMinimalBlock(sb *gloas.ElectraSignedBeaconBlock, specs map[string]any, step int) *gloas.ElectraSignedBeaconBlock {
	index := func(i gloas.ValidatorIndex) gloas.ValidatorIndex { return i / gloas.ValidatorIndex(step) }
	bitsMax := spec(specs, "MAX_VALIDATORS_PER_COMMITTEE") * spec(specs, "MAX_COMMITTEES_PER_SLOT")
	committees := spec(specs, "MAX_COMMITTEES_PER_SLOT")
	header := func(h *gloas.SignedBeaconBlockHeader) *gloas.SignedBeaconBlockHeader {
		m := *h.Message
		m.ProposerIndex = index(m.ProposerIndex)
		return &gloas.SignedBeaconBlockHeader{Message: &m, Signature: h.Signature}
	}
	indexed := func(a *gloas.ElectraIndexedAttestation) *gloas.ElectraIndexedAttestation {
		return &gloas.ElectraIndexedAttestation{AttestingIndices: head(a.AttestingIndices, bitsMax), Data: a.Data, Signature: a.Signature}
	}

	b := *sb.Message.Body
	b.ProposerSlashings = make([]*gloas.ProposerSlashing, 0, len(sb.Message.Body.ProposerSlashings))
	for _, s := range sb.Message.Body.ProposerSlashings {
		b.ProposerSlashings = append(b.ProposerSlashings, &gloas.ProposerSlashing{SignedHeader1: header(s.SignedHeader1), SignedHeader2: header(s.SignedHeader2)})
	}
	b.AttesterSlashings = make([]*gloas.ElectraAttesterSlashing, 0, len(sb.Message.Body.AttesterSlashings))
	for _, s := range sb.Message.Body.AttesterSlashings {
		b.AttesterSlashings = append(b.AttesterSlashings, &gloas.ElectraAttesterSlashing{Attestation1: indexed(s.Attestation1), Attestation2: indexed(s.Attestation2)})
	}
	b.Attestations = make([]*gloas.ElectraAttestation, 0, len(sb.Message.Body.Attestations))
	for _, a := range sb.Message.Body.Attestations {
		bits := a.AggregationBits
		if bits.Len() > uint64(bitsMax) {
			bits = bitfield.NewBitlist(uint64(bitsMax))
			for i := range uint64(bitsMax) {
				bits.SetBitAt(i, a.AggregationBits.BitAt(i))
			}
		}
		cb := bitvector(a.CommitteeBits, committees)
		if bitfield.Bitvector64(cb).Count() == 0 {
			cb[0] |= 1 // an attestation is of one committee at least
		}
		b.Attestations = append(b.Attestations, &gloas.ElectraAttestation{AggregationBits: bits, Data: a.Data, Signature: a.Signature, CommitteeBits: cb})
	}
	b.VoluntaryExits = make([]*gloas.SignedVoluntaryExit, 0, len(sb.Message.Body.VoluntaryExits))
	for _, e := range sb.Message.Body.VoluntaryExits {
		b.VoluntaryExits = append(b.VoluntaryExits, &gloas.SignedVoluntaryExit{Message: &gloas.VoluntaryExit{Epoch: e.Message.Epoch, ValidatorIndex: index(e.Message.ValidatorIndex)}, Signature: e.Signature})
	}
	b.SyncAggregate = &gloas.AltairSyncAggregate{SyncCommitteeBits: bitvector(sb.Message.Body.SyncAggregate.SyncCommitteeBits, spec(specs, "SYNC_COMMITTEE_SIZE")), SyncCommitteeSignature: sb.Message.Body.SyncAggregate.SyncCommitteeSignature}
	b.BLSToExecutionChanges = make([]*gloas.CapellaSignedBLSToExecutionChange, 0, len(sb.Message.Body.BLSToExecutionChanges))
	for _, c := range sb.Message.Body.BLSToExecutionChanges {
		m := *c.Message
		m.ValidatorIndex = index(m.ValidatorIndex)
		b.BLSToExecutionChanges = append(b.BLSToExecutionChanges, &gloas.CapellaSignedBLSToExecutionChange{Message: &m, Signature: c.Signature})
	}
	payload := *sb.Message.Body.ExecutionPayload
	payload.Withdrawals = make([]*gloas.CapellaWithdrawal, 0, spec(specs, "MAX_WITHDRAWALS_PER_PAYLOAD"))
	for _, w := range head(sb.Message.Body.ExecutionPayload.Withdrawals, spec(specs, "MAX_WITHDRAWALS_PER_PAYLOAD")) {
		c := *w
		c.ValidatorIndex = index(c.ValidatorIndex)
		payload.Withdrawals = append(payload.Withdrawals, &c)
	}
	b.ExecutionPayload = &payload

	m := *sb.Message
	m.ProposerIndex = index(m.ProposerIndex)
	m.Body = &b
	return &gloas.ElectraSignedBeaconBlock{Message: &m, Signature: sb.Signature}
}
