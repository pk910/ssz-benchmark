package gloas

import (
	binary "encoding/binary"
	"fmt"
	go_bitfield "github.com/OffchainLabs/go-bitfield"
	ssz "github.com/OffchainLabs/methodical-ssz/ssz"
)

func (c *AttestationData) SizeSSZ() int {
	size := 128

	return size
}

func (c *AttestationData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AttestationData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 1: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	// Field 2: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot[:]...)

	// Field 3: Source
	if c.Source == nil {
		c.Source = new(Checkpoint)
	}
	if dst, err = c.Source.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Source: %w", err)
	}

	// Field 4: Target
	if c.Target == nil {
		c.Target = new(Checkpoint)
	}
	if dst, err = c.Target.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Target: %w", err)
	}

	return dst, err
}

func (c *AttestationData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 128 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.Slot
	sszSlice1 := buf[8:16]   // c.Index
	sszSlice2 := buf[16:48]  // c.BeaconBlockRoot
	sszSlice3 := buf[48:88]  // c.Source
	sszSlice4 := buf[88:128] // c.Target

	// Field 0: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: BeaconBlockRoot
	copy(c.BeaconBlockRoot[:], sszSlice2)

	// Field 3: Source
	c.Source = new(Checkpoint)
	if err = c.Source.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Source: %w", err)
	}

	// Field 4: Target
	c.Target = new(Checkpoint)
	if err = c.Target.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Target: %w", err)
	}
	return err
}

func (c *AttestationData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AttestationData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	hh.PutUint64(c.Slot)
	// Field 1: Index
	hh.PutUint64(c.Index)
	// Field 2: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot[:])
	// Field 3: Source
	if err := c.Source.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Source: %w", err)
	}
	// Field 4: Target
	if err := c.Target.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Target: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *BeaconBlockHeader) SizeSSZ() int {
	size := 112

	return size
}

func (c *BeaconBlockHeader) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *BeaconBlockHeader) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 1: ProposerIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ProposerIndex)

	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentRoot[:]...)

	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot[:]...)

	// Field 4: BodyRoot
	if len(c.BodyRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BodyRoot[:]...)

	return dst, err
}

func (c *BeaconBlockHeader) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 112 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]    // c.Slot
	sszSlice1 := buf[8:16]   // c.ProposerIndex
	sszSlice2 := buf[16:48]  // c.ParentRoot
	sszSlice3 := buf[48:80]  // c.StateRoot
	sszSlice4 := buf[80:112] // c.BodyRoot

	// Field 0: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: ProposerIndex
	c.ProposerIndex = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: ParentRoot
	copy(c.ParentRoot[:], sszSlice2)

	// Field 3: StateRoot
	copy(c.StateRoot[:], sszSlice3)

	// Field 4: BodyRoot
	copy(c.BodyRoot[:], sszSlice4)
	return err
}

func (c *BeaconBlockHeader) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *BeaconBlockHeader) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	hh.PutUint64(c.Slot)
	// Field 1: ProposerIndex
	hh.PutUint64(c.ProposerIndex)
	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentRoot[:])
	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot[:])
	// Field 4: BodyRoot
	if len(c.BodyRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BodyRoot[:])
	hh.Merkleize(indx)
	return nil
}

func (c *Checkpoint) SizeSSZ() int {
	size := 40

	return size
}

func (c *Checkpoint) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Checkpoint) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Epoch
	dst = binary.LittleEndian.AppendUint64(dst, c.Epoch)

	// Field 1: Root
	if len(c.Root) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Root[:]...)

	return dst, err
}

func (c *Checkpoint) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 40 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Epoch
	sszSlice1 := buf[8:40] // c.Root

	// Field 0: Epoch
	c.Epoch = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Root
	copy(c.Root[:], sszSlice1)
	return err
}

func (c *Checkpoint) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Checkpoint) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Epoch
	hh.PutUint64(c.Epoch)
	// Field 1: Root
	if len(c.Root) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Root[:])
	hh.Merkleize(indx)
	return nil
}

func (c *Deposit) SizeSSZ() int {
	size := 1240

	return size
}

func (c *Deposit) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Deposit) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Proof
	if len(c.Proof) != 33 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Proof {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o...)
	}

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(DepositData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	return dst, err
}

func (c *Deposit) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 1240 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:1056]    // c.Proof
	sszSlice1 := buf[1056:1240] // c.Data

	// Field 0: Proof
	{
		c.Proof = make([][]byte, 33)
		for i := 0; i < 33; i++ {
			var tmp []byte

			tmpSlice := sszSlice0[i*32 : (1+i)*32]
			tmp = make([]byte, 0, 32)
			tmp = append(tmp, tmpSlice...)
			c.Proof[i] = tmp
		}
	}

	// Field 1: Data
	c.Data = new(DepositData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	return err
}

func (c *Deposit) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Deposit) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Proof
	{
		if len(c.Proof) != 33 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.Proof {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *DepositData) SizeSSZ() int {
	size := 184

	return size
}

func (c *DepositData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *DepositData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PublicKey[:]...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *DepositData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 184 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]   // c.PublicKey
	sszSlice1 := buf[48:80]  // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]  // c.Amount
	sszSlice3 := buf[88:184] // c.Signature

	// Field 0: PublicKey
	copy(c.PublicKey[:], sszSlice0)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	copy(c.Signature[:], sszSlice3)
	return err
}

func (c *DepositData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *DepositData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PublicKey[:])
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *ETH1Data) SizeSSZ() int {
	size := 72

	return size
}

func (c *ETH1Data) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ETH1Data) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: DepositRoot
	if len(c.DepositRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.DepositRoot[:]...)

	// Field 1: DepositCount
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositCount)

	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash...)

	return dst, err
}

func (c *ETH1Data) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 72 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.DepositRoot
	sszSlice1 := buf[32:40] // c.DepositCount
	sszSlice2 := buf[40:72] // c.BlockHash

	// Field 0: DepositRoot
	copy(c.DepositRoot[:], sszSlice0)

	// Field 1: DepositCount
	c.DepositCount = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: BlockHash
	c.BlockHash = make([]byte, 0, 32)
	c.BlockHash = append(c.BlockHash, sszSlice2...)
	return err
}

func (c *ETH1Data) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ETH1Data) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: DepositRoot
	if len(c.DepositRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.DepositRoot[:])
	// Field 1: DepositCount
	hh.PutUint64(c.DepositCount)
	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash)
	hh.Merkleize(indx)
	return nil
}

func (c *Fork) SizeSSZ() int {
	size := 16

	return size
}

func (c *Fork) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Fork) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: PreviousVersion
	if len(c.PreviousVersion) != 4 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PreviousVersion[:]...)

	// Field 1: CurrentVersion
	if len(c.CurrentVersion) != 4 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.CurrentVersion[:]...)

	// Field 2: Epoch
	dst = binary.LittleEndian.AppendUint64(dst, c.Epoch)

	return dst, err
}

func (c *Fork) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:4]  // c.PreviousVersion
	sszSlice1 := buf[4:8]  // c.CurrentVersion
	sszSlice2 := buf[8:16] // c.Epoch

	// Field 0: PreviousVersion
	copy(c.PreviousVersion[:], sszSlice0)

	// Field 1: CurrentVersion
	copy(c.CurrentVersion[:], sszSlice1)

	// Field 2: Epoch
	c.Epoch = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *Fork) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Fork) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: PreviousVersion
	if len(c.PreviousVersion) != 4 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PreviousVersion[:])
	// Field 1: CurrentVersion
	if len(c.CurrentVersion) != 4 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.CurrentVersion[:])
	// Field 2: Epoch
	hh.PutUint64(c.Epoch)
	hh.Merkleize(indx)
	return nil
}

func (c *ProposerSlashing) SizeSSZ() int {
	size := 416

	return size
}

func (c *ProposerSlashing) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ProposerSlashing) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SignedHeader1
	if c.SignedHeader1 == nil {
		c.SignedHeader1 = new(SignedBeaconBlockHeader)
	}
	if dst, err = c.SignedHeader1.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedHeader1: %w", err)
	}

	// Field 1: SignedHeader2
	if c.SignedHeader2 == nil {
		c.SignedHeader2 = new(SignedBeaconBlockHeader)
	}
	if dst, err = c.SignedHeader2.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedHeader2: %w", err)
	}

	return dst, err
}

func (c *ProposerSlashing) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 416 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:208]   // c.SignedHeader1
	sszSlice1 := buf[208:416] // c.SignedHeader2

	// Field 0: SignedHeader1
	c.SignedHeader1 = new(SignedBeaconBlockHeader)
	if err = c.SignedHeader1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("SignedHeader1: %w", err)
	}

	// Field 1: SignedHeader2
	c.SignedHeader2 = new(SignedBeaconBlockHeader)
	if err = c.SignedHeader2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("SignedHeader2: %w", err)
	}
	return err
}

func (c *ProposerSlashing) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ProposerSlashing) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SignedHeader1
	if err := c.SignedHeader1.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedHeader1: %w", err)
	}
	// Field 1: SignedHeader2
	if err := c.SignedHeader2.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedHeader2: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *SignedBeaconBlockHeader) SizeSSZ() int {
	size := 208

	return size
}

func (c *SignedBeaconBlockHeader) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedBeaconBlockHeader) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(BeaconBlockHeader)
	}
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *SignedBeaconBlockHeader) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 208 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:112]   // c.Message
	sszSlice1 := buf[112:208] // c.Signature

	// Field 0: Message
	c.Message = new(BeaconBlockHeader)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *SignedBeaconBlockHeader) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedBeaconBlockHeader) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *SignedVoluntaryExit) SizeSSZ() int {
	size := 112

	return size
}

func (c *SignedVoluntaryExit) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *SignedVoluntaryExit) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(VoluntaryExit)
	}
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *SignedVoluntaryExit) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 112 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:16]   // c.Message
	sszSlice1 := buf[16:112] // c.Signature

	// Field 0: Message
	c.Message = new(VoluntaryExit)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *SignedVoluntaryExit) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *SignedVoluntaryExit) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *Validator) SizeSSZ() int {
	size := 121

	return size
}

func (c *Validator) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *Validator) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PublicKey[:]...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: EffectiveBalance
	dst = binary.LittleEndian.AppendUint64(dst, c.EffectiveBalance)

	// Field 3: Slashed
	if c.Slashed {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 4: ActivationEligibilityEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.ActivationEligibilityEpoch)

	// Field 5: ActivationEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.ActivationEpoch)

	// Field 6: ExitEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.ExitEpoch)

	// Field 7: WithdrawableEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.WithdrawableEpoch)

	return dst, err
}

func (c *Validator) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 121 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]    // c.PublicKey
	sszSlice1 := buf[48:80]   // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]   // c.EffectiveBalance
	sszSlice3 := buf[88:89]   // c.Slashed
	sszSlice4 := buf[89:97]   // c.ActivationEligibilityEpoch
	sszSlice5 := buf[97:105]  // c.ActivationEpoch
	sszSlice6 := buf[105:113] // c.ExitEpoch
	sszSlice7 := buf[113:121] // c.WithdrawableEpoch

	// Field 0: PublicKey
	copy(c.PublicKey[:], sszSlice0)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: EffectiveBalance
	c.EffectiveBalance = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Slashed
	if sszSlice3[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice3[0] == 1 {
		c.Slashed = true
	} else {
		c.Slashed = false
	}

	// Field 4: ActivationEligibilityEpoch
	c.ActivationEligibilityEpoch = binary.LittleEndian.Uint64(sszSlice4)

	// Field 5: ActivationEpoch
	c.ActivationEpoch = binary.LittleEndian.Uint64(sszSlice5)

	// Field 6: ExitEpoch
	c.ExitEpoch = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: WithdrawableEpoch
	c.WithdrawableEpoch = binary.LittleEndian.Uint64(sszSlice7)
	return err
}

func (c *Validator) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *Validator) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PublicKey[:])
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: EffectiveBalance
	hh.PutUint64(c.EffectiveBalance)
	// Field 3: Slashed
	hh.PutBool(c.Slashed)
	// Field 4: ActivationEligibilityEpoch
	hh.PutUint64(c.ActivationEligibilityEpoch)
	// Field 5: ActivationEpoch
	hh.PutUint64(c.ActivationEpoch)
	// Field 6: ExitEpoch
	hh.PutUint64(c.ExitEpoch)
	// Field 7: WithdrawableEpoch
	hh.PutUint64(c.WithdrawableEpoch)
	hh.Merkleize(indx)
	return nil
}

func (c *VoluntaryExit) SizeSSZ() int {
	size := 16

	return size
}

func (c *VoluntaryExit) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *VoluntaryExit) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Epoch
	dst = binary.LittleEndian.AppendUint64(dst, c.Epoch)

	// Field 1: ValidatorIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ValidatorIndex)

	return dst, err
}

func (c *VoluntaryExit) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.Epoch
	sszSlice1 := buf[8:16] // c.ValidatorIndex

	// Field 0: Epoch
	c.Epoch = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: ValidatorIndex
	c.ValidatorIndex = binary.LittleEndian.Uint64(sszSlice1)
	return err
}

func (c *VoluntaryExit) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *VoluntaryExit) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Epoch
	hh.PutUint64(c.Epoch)
	// Field 1: ValidatorIndex
	hh.PutUint64(c.ValidatorIndex)
	hh.Merkleize(indx)
	return nil
}

func (c *AltairSyncAggregate) SizeSSZ() int {
	size := 160

	return size
}

func (c *AltairSyncAggregate) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AltairSyncAggregate) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SyncCommitteeBits
	if len([]byte(c.SyncCommitteeBits)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.SyncCommitteeBits)...)

	// Field 1: SyncCommitteeSignature
	if len(c.SyncCommitteeSignature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SyncCommitteeSignature[:]...)

	return dst, err
}

func (c *AltairSyncAggregate) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 160 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:64]   // c.SyncCommitteeBits
	sszSlice1 := buf[64:160] // c.SyncCommitteeSignature

	// Field 0: SyncCommitteeBits
	c.SyncCommitteeBits = make([]byte, 0, 64)
	c.SyncCommitteeBits = append(c.SyncCommitteeBits, go_bitfield.Bitvector512(sszSlice0)...)

	// Field 1: SyncCommitteeSignature
	copy(c.SyncCommitteeSignature[:], sszSlice1)
	return err
}

func (c *AltairSyncAggregate) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AltairSyncAggregate) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SyncCommitteeBits
	if len([]byte(c.SyncCommitteeBits)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.SyncCommitteeBits))
	// Field 1: SyncCommitteeSignature
	if len(c.SyncCommitteeSignature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SyncCommitteeSignature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *AltairSyncCommittee) SizeSSZ() int {
	size := 24624

	return size
}

func (c *AltairSyncCommittee) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *AltairSyncCommittee) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkeys
	if len(c.Pubkeys) != 512 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Pubkeys {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}

	// Field 1: AggregatePubkey
	if len(c.AggregatePubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.AggregatePubkey[:]...)

	return dst, err
}

func (c *AltairSyncCommittee) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 24624 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:24576]     // c.Pubkeys
	sszSlice1 := buf[24576:24624] // c.AggregatePubkey

	// Field 0: Pubkeys
	{
		c.Pubkeys = make([][48]byte, 512)
		for i := 0; i < 512; i++ {
			var tmp [48]byte

			tmpSlice := sszSlice0[i*48 : (1+i)*48]
			copy(tmp[:], tmpSlice)
			c.Pubkeys[i] = tmp
		}
	}

	// Field 1: AggregatePubkey
	copy(c.AggregatePubkey[:], sszSlice1)
	return err
}

func (c *AltairSyncCommittee) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *AltairSyncCommittee) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkeys
	{
		if len(c.Pubkeys) != 512 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.Pubkeys {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o[:])
		}
		hh.Merkleize(subIndx)
	}
	// Field 1: AggregatePubkey
	if len(c.AggregatePubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.AggregatePubkey[:])
	hh.Merkleize(indx)
	return nil
}

func (c *CapellaBLSToExecutionChange) SizeSSZ() int {
	size := 76

	return size
}

func (c *CapellaBLSToExecutionChange) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *CapellaBLSToExecutionChange) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: ValidatorIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ValidatorIndex)

	// Field 1: FromBLSPubkey
	if len(c.FromBLSPubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FromBLSPubkey[:]...)

	// Field 2: ToExecutionAddress
	if len(c.ToExecutionAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ToExecutionAddress[:]...)

	return dst, err
}

func (c *CapellaBLSToExecutionChange) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 76 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.ValidatorIndex
	sszSlice1 := buf[8:56]  // c.FromBLSPubkey
	sszSlice2 := buf[56:76] // c.ToExecutionAddress

	// Field 0: ValidatorIndex
	c.ValidatorIndex = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: FromBLSPubkey
	copy(c.FromBLSPubkey[:], sszSlice1)

	// Field 2: ToExecutionAddress
	copy(c.ToExecutionAddress[:], sszSlice2)
	return err
}

func (c *CapellaBLSToExecutionChange) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *CapellaBLSToExecutionChange) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ValidatorIndex
	hh.PutUint64(c.ValidatorIndex)
	// Field 1: FromBLSPubkey
	if len(c.FromBLSPubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FromBLSPubkey[:])
	// Field 2: ToExecutionAddress
	if len(c.ToExecutionAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ToExecutionAddress[:])
	hh.Merkleize(indx)
	return nil
}

func (c *CapellaHistoricalSummary) SizeSSZ() int {
	size := 64

	return size
}

func (c *CapellaHistoricalSummary) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *CapellaHistoricalSummary) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: BlockSummaryRoot
	if len(c.BlockSummaryRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockSummaryRoot[:]...)

	// Field 1: StateSummaryRoot
	if len(c.StateSummaryRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateSummaryRoot[:]...)

	return dst, err
}

func (c *CapellaHistoricalSummary) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 64 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.BlockSummaryRoot
	sszSlice1 := buf[32:64] // c.StateSummaryRoot

	// Field 0: BlockSummaryRoot
	copy(c.BlockSummaryRoot[:], sszSlice0)

	// Field 1: StateSummaryRoot
	copy(c.StateSummaryRoot[:], sszSlice1)
	return err
}

func (c *CapellaHistoricalSummary) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *CapellaHistoricalSummary) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: BlockSummaryRoot
	if len(c.BlockSummaryRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockSummaryRoot[:])
	// Field 1: StateSummaryRoot
	if len(c.StateSummaryRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateSummaryRoot[:])
	hh.Merkleize(indx)
	return nil
}

func (c *CapellaSignedBLSToExecutionChange) SizeSSZ() int {
	size := 172

	return size
}

func (c *CapellaSignedBLSToExecutionChange) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *CapellaSignedBLSToExecutionChange) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(CapellaBLSToExecutionChange)
	}
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *CapellaSignedBLSToExecutionChange) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 172 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:76]   // c.Message
	sszSlice1 := buf[76:172] // c.Signature

	// Field 0: Message
	c.Message = new(CapellaBLSToExecutionChange)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *CapellaSignedBLSToExecutionChange) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *CapellaSignedBLSToExecutionChange) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *CapellaWithdrawal) SizeSSZ() int {
	size := 44

	return size
}

func (c *CapellaWithdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *CapellaWithdrawal) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	// Field 1: ValidatorIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ValidatorIndex)

	// Field 2: Address
	if len(c.Address) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Address[:]...)

	// Field 3: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	return dst, err
}

func (c *CapellaWithdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 44 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Index
	sszSlice1 := buf[8:16]  // c.ValidatorIndex
	sszSlice2 := buf[16:36] // c.Address
	sszSlice3 := buf[36:44] // c.Amount

	// Field 0: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: ValidatorIndex
	c.ValidatorIndex = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: Address
	copy(c.Address[:], sszSlice2)

	// Field 3: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice3)
	return err
}

func (c *CapellaWithdrawal) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *CapellaWithdrawal) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Index
	hh.PutUint64(c.Index)
	// Field 1: ValidatorIndex
	hh.PutUint64(c.ValidatorIndex)
	// Field 2: Address
	if len(c.Address) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Address[:])
	// Field 3: Amount
	hh.PutUint64(c.Amount)
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraConsolidationRequest) SizeSSZ() int {
	size := 116

	return size
}

func (c *ElectraConsolidationRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraConsolidationRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress[:]...)

	// Field 1: SourcePubkey
	if len(c.SourcePubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourcePubkey[:]...)

	// Field 2: TargetPubkey
	if len(c.TargetPubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.TargetPubkey[:]...)

	return dst, err
}

func (c *ElectraConsolidationRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 116 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]   // c.SourceAddress
	sszSlice1 := buf[20:68]  // c.SourcePubkey
	sszSlice2 := buf[68:116] // c.TargetPubkey

	// Field 0: SourceAddress
	copy(c.SourceAddress[:], sszSlice0)

	// Field 1: SourcePubkey
	copy(c.SourcePubkey[:], sszSlice1)

	// Field 2: TargetPubkey
	copy(c.TargetPubkey[:], sszSlice2)
	return err
}

func (c *ElectraConsolidationRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraConsolidationRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress[:])
	// Field 1: SourcePubkey
	if len(c.SourcePubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourcePubkey[:])
	// Field 2: TargetPubkey
	if len(c.TargetPubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.TargetPubkey[:])
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraDepositRequest) SizeSSZ() int {
	size := 192

	return size
}

func (c *ElectraDepositRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraDepositRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey[:]...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 4: Index
	dst = binary.LittleEndian.AppendUint64(dst, c.Index)

	return dst, err
}

func (c *ElectraDepositRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 192 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]    // c.Pubkey
	sszSlice1 := buf[48:80]   // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]   // c.Amount
	sszSlice3 := buf[88:184]  // c.Signature
	sszSlice4 := buf[184:192] // c.Index

	// Field 0: Pubkey
	copy(c.Pubkey[:], sszSlice0)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	copy(c.Signature[:], sszSlice3)

	// Field 4: Index
	c.Index = binary.LittleEndian.Uint64(sszSlice4)
	return err
}

func (c *ElectraDepositRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraDepositRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey[:])
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	// Field 4: Index
	hh.PutUint64(c.Index)
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraPendingDeposit) SizeSSZ() int {
	size := 192

	return size
}

func (c *ElectraPendingDeposit) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraPendingDeposit) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey[:]...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 4: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	return dst, err
}

func (c *ElectraPendingDeposit) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 192 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]    // c.Pubkey
	sszSlice1 := buf[48:80]   // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]   // c.Amount
	sszSlice3 := buf[88:184]  // c.Signature
	sszSlice4 := buf[184:192] // c.Slot

	// Field 0: Pubkey
	copy(c.Pubkey[:], sszSlice0)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	copy(c.Signature[:], sszSlice3)

	// Field 4: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice4)
	return err
}

func (c *ElectraPendingDeposit) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraPendingDeposit) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey[:])
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	// Field 4: Slot
	hh.PutUint64(c.Slot)
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraPendingConsolidation) SizeSSZ() int {
	size := 16

	return size
}

func (c *ElectraPendingConsolidation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraPendingConsolidation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.SourceIndex)

	// Field 1: TargetIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.TargetIndex)

	return dst, err
}

func (c *ElectraPendingConsolidation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 16 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]  // c.SourceIndex
	sszSlice1 := buf[8:16] // c.TargetIndex

	// Field 0: SourceIndex
	c.SourceIndex = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: TargetIndex
	c.TargetIndex = binary.LittleEndian.Uint64(sszSlice1)
	return err
}

func (c *ElectraPendingConsolidation) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraPendingConsolidation) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceIndex
	hh.PutUint64(c.SourceIndex)
	// Field 1: TargetIndex
	hh.PutUint64(c.TargetIndex)
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraPendingPartialWithdrawal) SizeSSZ() int {
	size := 24

	return size
}

func (c *ElectraPendingPartialWithdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraPendingPartialWithdrawal) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: ValidatorIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ValidatorIndex)

	// Field 1: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 2: WithdrawableEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.WithdrawableEpoch)

	return dst, err
}

func (c *ElectraPendingPartialWithdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 24 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.ValidatorIndex
	sszSlice1 := buf[8:16]  // c.Amount
	sszSlice2 := buf[16:24] // c.WithdrawableEpoch

	// Field 0: ValidatorIndex
	c.ValidatorIndex = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: WithdrawableEpoch
	c.WithdrawableEpoch = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *ElectraPendingPartialWithdrawal) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraPendingPartialWithdrawal) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ValidatorIndex
	hh.PutUint64(c.ValidatorIndex)
	// Field 1: Amount
	hh.PutUint64(c.Amount)
	// Field 2: WithdrawableEpoch
	hh.PutUint64(c.WithdrawableEpoch)
	hh.Merkleize(indx)
	return nil
}

func (c *ElectraWithdrawalRequest) SizeSSZ() int {
	size := 76

	return size
}

func (c *ElectraWithdrawalRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *ElectraWithdrawalRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress[:]...)

	// Field 1: ValidatorPubkey
	if len(c.ValidatorPubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ValidatorPubkey[:]...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	return dst, err
}

func (c *ElectraWithdrawalRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 76 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.SourceAddress
	sszSlice1 := buf[20:68] // c.ValidatorPubkey
	sszSlice2 := buf[68:76] // c.Amount

	// Field 0: SourceAddress
	copy(c.SourceAddress[:], sszSlice0)

	// Field 1: ValidatorPubkey
	copy(c.ValidatorPubkey[:], sszSlice1)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *ElectraWithdrawalRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *ElectraWithdrawalRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress[:])
	// Field 1: ValidatorPubkey
	if len(c.ValidatorPubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ValidatorPubkey[:])
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	hh.Merkleize(indx)
	return nil
}

func (c *GloasAttestation) SizeSSZ() int {
	size := 236
	size += len(c.AggregationBits)
	return size
}

func (c *GloasAttestation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasAttestation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 236

	// Field 0: AggregationBits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AggregationBits)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 3: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 8 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.CommitteeBits)...)

	// Field 0: AggregationBits
	dst = append(dst, c.AggregationBits...)
	return dst, err
}

func (c *GloasAttestation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 236 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132]   // c.Data
	sszSlice2 := buf[132:228] // c.Signature
	sszSlice3 := buf[228:236] // c.CommitteeBits

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AggregationBits
	if sszVarOffset0 != 236 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AggregationBits

	// Field 0: AggregationBits
	if err = ssz.ValidateProgressiveBitlist(sszSlice0); err != nil {
		return fmt.Errorf("AggregationBits: %w", err)
	}
	c.AggregationBits = append([]byte{}, go_bitfield.Bitlist(sszSlice0)...)

	// Field 1: Data
	c.Data = new(AttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	copy(c.Signature[:], sszSlice2)

	// Field 3: CommitteeBits
	c.CommitteeBits = make([]byte, 0, 8)
	c.CommitteeBits = append(c.CommitteeBits, go_bitfield.Bitvector64(sszSlice3)...)
	return err
}

func (c *GloasAttestation) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasAttestation) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasAttestation = []byte{0b00001111}

func (c *GloasAttestation) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasAttestation) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregationBits
	if len(c.AggregationBits) == 0 {
		return ssz.ErrEmptyBitlist
	}
	hh.PutProgressiveBitlist(c.AggregationBits)
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	// Field 3: CommitteeBits
	if len([]byte(c.CommitteeBits)) != 8 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.CommitteeBits))
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasAttestation)
	return nil
}

func (c *GloasAttesterSlashing) SizeSSZ() int {
	size := 8
	if c.Attestation1 == nil {
		c.Attestation1 = new(GloasIndexedAttestation)
	}
	size += c.Attestation1.SizeSSZ()
	if c.Attestation2 == nil {
		c.Attestation2 = new(GloasIndexedAttestation)
	}
	size += c.Attestation2.SizeSSZ()
	return size
}

func (c *GloasAttesterSlashing) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasAttesterSlashing) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 8

	// Field 0: Attestation1
	if c.Attestation1 == nil {
		c.Attestation1 = new(GloasIndexedAttestation)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation1.SizeSSZ()

	// Field 1: Attestation2
	if c.Attestation2 == nil {
		c.Attestation2 = new(GloasIndexedAttestation)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Attestation2.SizeSSZ()

	// Field 0: Attestation1
	if dst, err = c.Attestation1.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Attestation1: %w", err)
	}

	// Field 1: Attestation2
	if dst, err = c.Attestation2.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Attestation2: %w", err)
	}
	return dst, err
}

func (c *GloasAttesterSlashing) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 8 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Attestation1
	if sszVarOffset0 != 8 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Attestation2
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Attestation1
	sszSlice1 := buf[sszVarOffset1:]              // c.Attestation2

	// Field 0: Attestation1
	c.Attestation1 = new(GloasIndexedAttestation)
	if err = c.Attestation1.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Attestation1: %w", err)
	}

	// Field 1: Attestation2
	c.Attestation2 = new(GloasIndexedAttestation)
	if err = c.Attestation2.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Attestation2: %w", err)
	}
	return err
}

func (c *GloasAttesterSlashing) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasAttesterSlashing) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Attestation1
	if err := c.Attestation1.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Attestation1: %w", err)
	}
	// Field 1: Attestation2
	if err := c.Attestation2.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Attestation2: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBeaconBlock) SizeSSZ() int {
	size := 84
	if c.Body == nil {
		c.Body = new(GloasBeaconBlockBody)
	}
	size += c.Body.SizeSSZ()
	return size
}

func (c *GloasBeaconBlock) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 84

	// Field 0: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 1: ProposerIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ProposerIndex)

	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentRoot[:]...)

	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot[:]...)

	// Field 4: Body
	if c.Body == nil {
		c.Body = new(GloasBeaconBlockBody)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Body.SizeSSZ()

	// Field 4: Body
	if dst, err = c.Body.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Body: %w", err)
	}
	return dst, err
}

func (c *GloasBeaconBlock) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 84 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Slot
	sszSlice1 := buf[8:16]  // c.ProposerIndex
	sszSlice2 := buf[16:48] // c.ParentRoot
	sszSlice3 := buf[48:80] // c.StateRoot

	sszVarOffset4 := ssz.ReadOffset(buf[80:84]) // c.Body
	if sszVarOffset4 != 84 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset4 > size {
		return ssz.ErrOffset
	}
	sszSlice4 := buf[sszVarOffset4:] // c.Body

	// Field 0: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: ProposerIndex
	c.ProposerIndex = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: ParentRoot
	copy(c.ParentRoot[:], sszSlice2)

	// Field 3: StateRoot
	copy(c.StateRoot[:], sszSlice3)

	// Field 4: Body
	c.Body = new(GloasBeaconBlockBody)
	if err = c.Body.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	return err
}

func (c *GloasBeaconBlock) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBeaconBlock) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Slot
	hh.PutUint64(c.Slot)
	// Field 1: ProposerIndex
	hh.PutUint64(c.ProposerIndex)
	// Field 2: ParentRoot
	if len(c.ParentRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentRoot[:])
	// Field 3: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot[:])
	// Field 4: Body
	if err := c.Body.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Body: %w", err)
	}
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBeaconBlockBody) SizeSSZ() int {
	size := 396
	size += len(c.ProposerSlashings) * 416
	for _, o := range c.AttesterSlashings {
		size += 4
		size += o.SizeSSZ()
	}
	for _, o := range c.Attestations {
		size += 4
		size += o.SizeSSZ()
	}
	size += len(c.Deposits) * 1240
	size += len(c.VoluntaryExits) * 112
	size += len(c.BLSToExecutionChanges) * 172
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(GloasSignedExecutionPayloadBid)
	}
	size += c.SignedExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadAttestations) * 202
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(GloasExecutionRequests)
	}
	size += c.ParentExecutionRequests.SizeSSZ()
	return size
}

func (c *GloasBeaconBlockBody) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBeaconBlockBody) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 396

	// Field 0: RANDAOReveal
	if len(c.RANDAOReveal) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.RANDAOReveal[:]...)

	// Field 1: ETH1Data
	if c.ETH1Data == nil {
		c.ETH1Data = new(ETH1Data)
	}
	if dst, err = c.ETH1Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ETH1Data: %w", err)
	}

	// Field 2: Graffiti
	if len(c.Graffiti) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Graffiti[:]...)

	// Field 3: ProposerSlashings
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ProposerSlashings) * 416

	// Field 4: AttesterSlashings
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.AttesterSlashings {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 5: Attestations
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Attestations {
		offset += 4
		offset += o.SizeSSZ()
	}

	// Field 6: Deposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Deposits) * 1240

	// Field 7: VoluntaryExits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.VoluntaryExits) * 112

	// Field 8: SyncAggregate
	if c.SyncAggregate == nil {
		c.SyncAggregate = new(AltairSyncAggregate)
	}
	if dst, err = c.SyncAggregate.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 9: BLSToExecutionChanges
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BLSToExecutionChanges) * 172

	// Field 10: SignedExecutionPayloadBid
	if c.SignedExecutionPayloadBid == nil {
		c.SignedExecutionPayloadBid = new(GloasSignedExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.SignedExecutionPayloadBid.SizeSSZ()

	// Field 11: PayloadAttestations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadAttestations) * 202

	// Field 12: ParentExecutionRequests
	if c.ParentExecutionRequests == nil {
		c.ParentExecutionRequests = new(GloasExecutionRequests)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ParentExecutionRequests.SizeSSZ()

	// Field 3: ProposerSlashings
	for _, o := range c.ProposerSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ProposerSlashings: %w", err)
		}
	}

	// Field 4: AttesterSlashings
	{
		offset = 4 * len(c.AttesterSlashings)
		for _, o := range c.AttesterSlashings {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.AttesterSlashings {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("AttesterSlashings: %w", err)
		}
	}

	// Field 5: Attestations
	{
		offset = 4 * len(c.Attestations)
		for _, o := range c.Attestations {
			dst = ssz.WriteOffset(dst, offset)
			offset += o.SizeSSZ()
		}
	}
	for _, o := range c.Attestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Attestations: %w", err)
		}
	}

	// Field 6: Deposits
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 7: VoluntaryExits
	for _, o := range c.VoluntaryExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("VoluntaryExits: %w", err)
		}
	}

	// Field 9: BLSToExecutionChanges
	for _, o := range c.BLSToExecutionChanges {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BLSToExecutionChanges: %w", err)
		}
	}

	// Field 10: SignedExecutionPayloadBid
	if dst, err = c.SignedExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 11: PayloadAttestations
	for _, o := range c.PayloadAttestations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadAttestations: %w", err)
		}
	}

	// Field 12: ParentExecutionRequests
	if dst, err = c.ParentExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *GloasBeaconBlockBody) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 396 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:96]    // c.RANDAOReveal
	sszSlice1 := buf[96:168]  // c.ETH1Data
	sszSlice2 := buf[168:200] // c.Graffiti
	sszSlice8 := buf[220:380] // c.SyncAggregate

	sszVarOffset3 := ssz.ReadOffset(buf[200:204]) // c.ProposerSlashings
	if sszVarOffset3 != 396 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset3 > size {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[204:208]) // c.AttesterSlashings
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszVarOffset5 := ssz.ReadOffset(buf[208:212]) // c.Attestations
	if sszVarOffset5 > size || sszVarOffset5 < sszVarOffset4 {
		return ssz.ErrOffset
	}
	sszVarOffset6 := ssz.ReadOffset(buf[212:216]) // c.Deposits
	if sszVarOffset6 > size || sszVarOffset6 < sszVarOffset5 {
		return ssz.ErrOffset
	}
	sszVarOffset7 := ssz.ReadOffset(buf[216:220]) // c.VoluntaryExits
	if sszVarOffset7 > size || sszVarOffset7 < sszVarOffset6 {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[380:384]) // c.BLSToExecutionChanges
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset10 := ssz.ReadOffset(buf[384:388]) // c.SignedExecutionPayloadBid
	if sszVarOffset10 > size || sszVarOffset10 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[388:392]) // c.PayloadAttestations
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[392:396]) // c.ParentExecutionRequests
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
		return ssz.ErrOffset
	}
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4]    // c.ProposerSlashings
	sszSlice4 := buf[sszVarOffset4:sszVarOffset5]    // c.AttesterSlashings
	sszSlice5 := buf[sszVarOffset5:sszVarOffset6]    // c.Attestations
	sszSlice6 := buf[sszVarOffset6:sszVarOffset7]    // c.Deposits
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.VoluntaryExits
	sszSlice9 := buf[sszVarOffset9:sszVarOffset10]   // c.BLSToExecutionChanges
	sszSlice10 := buf[sszVarOffset10:sszVarOffset11] // c.SignedExecutionPayloadBid
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.PayloadAttestations
	sszSlice12 := buf[sszVarOffset12:]               // c.ParentExecutionRequests

	// Field 0: RANDAOReveal
	copy(c.RANDAOReveal[:], sszSlice0)

	// Field 1: ETH1Data
	c.ETH1Data = new(ETH1Data)
	if err = c.ETH1Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ETH1Data: %w", err)
	}

	// Field 2: Graffiti
	copy(c.Graffiti[:], sszSlice2)

	// Field 3: ProposerSlashings
	{
		if len(sszSlice3)%416 != 0 {
			return fmt.Errorf("misaligned bytes: c.ProposerSlashings length is %d, which is not a multiple of 416: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 416
		c.ProposerSlashings = make([]*ProposerSlashing, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ProposerSlashing
			tmp = new(ProposerSlashing)
			tmpSlice := sszSlice3[i*416 : (1+i)*416]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
			c.ProposerSlashings[i] = tmp
		}
	}

	// Field 4: AttesterSlashings
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice4) > 3 {
			startOffset := ssz.ReadOffset(sszSlice4[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.AttesterSlashings")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.AttesterSlashings, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			totalVarBytes := uint64(len(sszSlice4))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*GloasAttesterSlashing, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *GloasAttesterSlashing
				tmp = new(GloasAttesterSlashing)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice4[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.AttesterSlashings", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.AttesterSlashings", endOffset, startOffset)
				}
				tmpSlice = sszSlice4[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("AttesterSlashings: %w", err)
				}
				c.AttesterSlashings[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice4) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.AttesterSlashings")
			}
			c.AttesterSlashings = make([]*GloasAttesterSlashing, 0)
		}
	}

	// Field 5: Attestations
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice5) > 3 {
			startOffset := ssz.ReadOffset(sszSlice5[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Attestations")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Attestations, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			totalVarBytes := uint64(len(sszSlice5))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*GloasAttestation, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp *GloasAttestation
				tmp = new(GloasAttestation)
				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice5[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Attestations", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Attestations", endOffset, startOffset)
				}
				tmpSlice = sszSlice5[startOffset:endOffset]
				if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
					return fmt.Errorf("Attestations: %w", err)
				}
				c.Attestations[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice5) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Attestations")
			}
			c.Attestations = make([]*GloasAttestation, 0)
		}
	}

	// Field 6: Deposits
	{
		if len(sszSlice6)%1240 != 0 {
			return fmt.Errorf("misaligned bytes: c.Deposits length is %d, which is not a multiple of 1240: %w", len(sszSlice6), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice6) / 1240
		c.Deposits = make([]*Deposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Deposit
			tmp = new(Deposit)
			tmpSlice := sszSlice6[i*1240 : (1+i)*1240]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
			c.Deposits[i] = tmp
		}
	}

	// Field 7: VoluntaryExits
	{
		if len(sszSlice7)%112 != 0 {
			return fmt.Errorf("misaligned bytes: c.VoluntaryExits length is %d, which is not a multiple of 112: %w", len(sszSlice7), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice7) / 112
		c.VoluntaryExits = make([]*SignedVoluntaryExit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *SignedVoluntaryExit
			tmp = new(SignedVoluntaryExit)
			tmpSlice := sszSlice7[i*112 : (1+i)*112]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
			c.VoluntaryExits[i] = tmp
		}
	}

	// Field 8: SyncAggregate
	c.SyncAggregate = new(AltairSyncAggregate)
	if err = c.SyncAggregate.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}

	// Field 9: BLSToExecutionChanges
	{
		if len(sszSlice9)%172 != 0 {
			return fmt.Errorf("misaligned bytes: c.BLSToExecutionChanges length is %d, which is not a multiple of 172: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 172
		c.BLSToExecutionChanges = make([]*CapellaSignedBLSToExecutionChange, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *CapellaSignedBLSToExecutionChange
			tmp = new(CapellaSignedBLSToExecutionChange)
			tmpSlice := sszSlice9[i*172 : (1+i)*172]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BLSToExecutionChanges: %w", err)
			}
			c.BLSToExecutionChanges[i] = tmp
		}
	}

	// Field 10: SignedExecutionPayloadBid
	c.SignedExecutionPayloadBid = new(GloasSignedExecutionPayloadBid)
	if err = c.SignedExecutionPayloadBid.UnmarshalSSZ(sszSlice10); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}

	// Field 11: PayloadAttestations
	{
		if len(sszSlice11)%202 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadAttestations length is %d, which is not a multiple of 202: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 202
		c.PayloadAttestations = make([]*GloasPayloadAttestation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *GloasPayloadAttestation
			tmp = new(GloasPayloadAttestation)
			tmpSlice := sszSlice11[i*202 : (1+i)*202]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
			c.PayloadAttestations[i] = tmp
		}
	}

	// Field 12: ParentExecutionRequests
	c.ParentExecutionRequests = new(GloasExecutionRequests)
	if err = c.ParentExecutionRequests.UnmarshalSSZ(sszSlice12); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	return err
}

func (c *GloasBeaconBlockBody) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasBeaconBlockBody) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasBeaconBlockBody = []byte{0b11111111, 0b00011111}

func (c *GloasBeaconBlockBody) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBeaconBlockBody) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: RANDAOReveal
	if len(c.RANDAOReveal) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.RANDAOReveal[:])
	// Field 1: ETH1Data
	if err := c.ETH1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ETH1Data: %w", err)
	}
	// Field 2: Graffiti
	if len(c.Graffiti) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Graffiti[:])
	// Field 3: ProposerSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.ProposerSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ProposerSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.ProposerSlashings)))
	}
	// Field 4: AttesterSlashings
	{
		subIndx := hh.Index()
		for _, o := range c.AttesterSlashings {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("AttesterSlashings: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttesterSlashings)))
	}
	// Field 5: Attestations
	{
		subIndx := hh.Index()
		for _, o := range c.Attestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Attestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Attestations)))
	}
	// Field 6: Deposits
	{
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Deposits)))
	}
	// Field 7: VoluntaryExits
	{
		subIndx := hh.Index()
		for _, o := range c.VoluntaryExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("VoluntaryExits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.VoluntaryExits)))
	}
	// Field 8: SyncAggregate
	if err := c.SyncAggregate.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SyncAggregate: %w", err)
	}
	// Field 9: BLSToExecutionChanges
	{
		subIndx := hh.Index()
		for _, o := range c.BLSToExecutionChanges {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BLSToExecutionChanges: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BLSToExecutionChanges)))
	}
	// Field 10: SignedExecutionPayloadBid
	if err := c.SignedExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("SignedExecutionPayloadBid: %w", err)
	}
	// Field 11: PayloadAttestations
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadAttestations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadAttestations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadAttestations)))
	}
	// Field 12: ParentExecutionRequests
	if err := c.ParentExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ParentExecutionRequests: %w", err)
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasBeaconBlockBody)
	return nil
}

func (c *GloasBeaconState) SizeSSZ() int {
	size := 3134845
	size += len(c.HistoricalRoots) * 32
	size += len(c.ETH1DataVotes) * 72
	size += len(c.Validators) * 121
	size += len(c.Balances) * 8
	size += len(c.PreviousEpochParticipation)
	size += len(c.CurrentEpochParticipation)
	size += len(c.InactivityScores) * 8
	size += len(c.HistoricalSummaries) * 64
	size += len(c.PendingDeposits) * 192
	size += len(c.PendingPartialWithdrawals) * 24
	size += len(c.PendingConsolidations) * 16
	size += len(c.Builders) * 93
	size += len(c.BuilderPendingWithdrawals) * 36
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(GloasExecutionPayloadBid)
	}
	size += c.LatestExecutionPayloadBid.SizeSSZ()
	size += len(c.PayloadExpectedWithdrawals) * 44
	return size
}

func (c *GloasBeaconState) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBeaconState) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 3134845

	// Field 0: GenesisTime
	dst = binary.LittleEndian.AppendUint64(dst, c.GenesisTime)

	// Field 1: GenesisValidatorsRoot
	if len(c.GenesisValidatorsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.GenesisValidatorsRoot[:]...)

	// Field 2: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 3: Fork
	if c.Fork == nil {
		c.Fork = new(Fork)
	}
	if dst, err = c.Fork.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Fork: %w", err)
	}

	// Field 4: LatestBlockHeader
	if c.LatestBlockHeader == nil {
		c.LatestBlockHeader = new(BeaconBlockHeader)
	}
	if dst, err = c.LatestBlockHeader.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestBlockHeader: %w", err)
	}

	// Field 5: BlockRoots
	if len(c.BlockRoots) != 8192 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BlockRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}

	// Field 6: StateRoots
	if len(c.StateRoots) != 8192 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.StateRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}

	// Field 7: HistoricalRoots
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalRoots) * 32

	// Field 8: ETH1Data
	if c.ETH1Data == nil {
		c.ETH1Data = new(ETH1Data)
	}
	if dst, err = c.ETH1Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ETH1Data: %w", err)
	}

	// Field 9: ETH1DataVotes
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ETH1DataVotes) * 72

	// Field 10: ETH1DepositIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ETH1DepositIndex)

	// Field 11: Validators
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Validators) * 121

	// Field 12: Balances
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Balances) * 8

	// Field 13: RANDAOMixes
	if len(c.RANDAOMixes) != 65536 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.RANDAOMixes {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}

	// Field 14: Slashings
	if len(c.Slashings) != 8192 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.Slashings {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 15: PreviousEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PreviousEpochParticipation)

	// Field 16: CurrentEpochParticipation
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.CurrentEpochParticipation)

	// Field 17: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.JustificationBits)...)

	// Field 18: PreviousJustifiedCheckpoint
	if c.PreviousJustifiedCheckpoint == nil {
		c.PreviousJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.PreviousJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 19: CurrentJustifiedCheckpoint
	if c.CurrentJustifiedCheckpoint == nil {
		c.CurrentJustifiedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.CurrentJustifiedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 20: FinalizedCheckpoint
	if c.FinalizedCheckpoint == nil {
		c.FinalizedCheckpoint = new(Checkpoint)
	}
	if dst, err = c.FinalizedCheckpoint.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 21: InactivityScores
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.InactivityScores) * 8

	// Field 22: CurrentSyncCommittee
	if c.CurrentSyncCommittee == nil {
		c.CurrentSyncCommittee = new(AltairSyncCommittee)
	}
	if dst, err = c.CurrentSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 23: NextSyncCommittee
	if c.NextSyncCommittee == nil {
		c.NextSyncCommittee = new(AltairSyncCommittee)
	}
	if dst, err = c.NextSyncCommittee.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 24: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LatestBlockHash[:]...)

	// Field 25: NextWithdrawalIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalIndex)

	// Field 26: NextWithdrawalValidatorIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalValidatorIndex)

	// Field 27: HistoricalSummaries
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.HistoricalSummaries) * 64

	// Field 28: DepositRequestsStartIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositRequestsStartIndex)

	// Field 29: DepositBalanceToConsume
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositBalanceToConsume)

	// Field 30: ExitBalanceToConsume
	dst = binary.LittleEndian.AppendUint64(dst, c.ExitBalanceToConsume)

	// Field 31: EarliestExitEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.EarliestExitEpoch)

	// Field 32: ConsolidationBalanceToConsume
	dst = binary.LittleEndian.AppendUint64(dst, c.ConsolidationBalanceToConsume)

	// Field 33: EarliestConsolidationEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.EarliestConsolidationEpoch)

	// Field 34: PendingDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingDeposits) * 192

	// Field 35: PendingPartialWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingPartialWithdrawals) * 24

	// Field 36: PendingConsolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PendingConsolidations) * 16

	// Field 37: ProposerLookahead
	if len(c.ProposerLookahead) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.ProposerLookahead {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 38: Builders
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Builders) * 93

	// Field 39: NextWithdrawalBuilderIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.NextWithdrawalBuilderIndex)

	// Field 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionPayloadAvailability...)

	// Field 41: BuilderPendingPayments
	if len(c.BuilderPendingPayments) != 64 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.BuilderPendingPayments {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingPayments: %w", err)
		}
	}

	// Field 42: BuilderPendingWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderPendingWithdrawals) * 36

	// Field 43: LatestExecutionPayloadBid
	if c.LatestExecutionPayloadBid == nil {
		c.LatestExecutionPayloadBid = new(GloasExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.LatestExecutionPayloadBid.SizeSSZ()

	// Field 44: PayloadExpectedWithdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.PayloadExpectedWithdrawals) * 44

	// Field 45: PTCWindow
	if len(c.PTCWindow) != 96 {
		return nil, ssz.ErrBytesLength
	}
	for _, o := range c.PTCWindow {
		if len(o) != 512 {
			return nil, ssz.ErrBytesLength
		}
		for _, oo := range o {
			dst = binary.LittleEndian.AppendUint64(dst, oo)
		}
	}

	// Field 7: HistoricalRoots
	if len(c.HistoricalRoots) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalRoots {
		if len(o) != 32 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}

	// Field 9: ETH1DataVotes
	if len(c.ETH1DataVotes) > 2048 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.ETH1DataVotes {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("ETH1DataVotes: %w", err)
		}
	}

	// Field 11: Validators
	for _, o := range c.Validators {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Validators: %w", err)
		}
	}

	// Field 12: Balances
	for _, o := range c.Balances {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 15: PreviousEpochParticipation
	dst = append(dst, c.PreviousEpochParticipation...)

	// Field 16: CurrentEpochParticipation
	dst = append(dst, c.CurrentEpochParticipation...)

	// Field 21: InactivityScores
	for _, o := range c.InactivityScores {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}

	// Field 27: HistoricalSummaries
	if len(c.HistoricalSummaries) > 16777216 {
		return nil, ssz.ErrListTooBig
	}
	for _, o := range c.HistoricalSummaries {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("HistoricalSummaries: %w", err)
		}
	}

	// Field 34: PendingDeposits
	for _, o := range c.PendingDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingDeposits: %w", err)
		}
	}

	// Field 35: PendingPartialWithdrawals
	for _, o := range c.PendingPartialWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingPartialWithdrawals: %w", err)
		}
	}

	// Field 36: PendingConsolidations
	for _, o := range c.PendingConsolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PendingConsolidations: %w", err)
		}
	}

	// Field 38: Builders
	for _, o := range c.Builders {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Builders: %w", err)
		}
	}

	// Field 42: BuilderPendingWithdrawals
	for _, o := range c.BuilderPendingWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderPendingWithdrawals: %w", err)
		}
	}

	// Field 43: LatestExecutionPayloadBid
	if dst, err = c.LatestExecutionPayloadBid.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 44: PayloadExpectedWithdrawals
	for _, o := range c.PayloadExpectedWithdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
		}
	}
	return dst, err
}

func (c *GloasBeaconState) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 3134845 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]              // c.GenesisTime
	sszSlice1 := buf[8:40]             // c.GenesisValidatorsRoot
	sszSlice2 := buf[40:48]            // c.Slot
	sszSlice3 := buf[48:64]            // c.Fork
	sszSlice4 := buf[64:176]           // c.LatestBlockHeader
	sszSlice5 := buf[176:262320]       // c.BlockRoots
	sszSlice6 := buf[262320:524464]    // c.StateRoots
	sszSlice8 := buf[524468:524540]    // c.ETH1Data
	sszSlice10 := buf[524544:524552]   // c.ETH1DepositIndex
	sszSlice13 := buf[524560:2621712]  // c.RANDAOMixes
	sszSlice14 := buf[2621712:2687248] // c.Slashings
	sszSlice17 := buf[2687256:2687257] // c.JustificationBits
	sszSlice18 := buf[2687257:2687297] // c.PreviousJustifiedCheckpoint
	sszSlice19 := buf[2687297:2687337] // c.CurrentJustifiedCheckpoint
	sszSlice20 := buf[2687337:2687377] // c.FinalizedCheckpoint
	sszSlice22 := buf[2687381:2712005] // c.CurrentSyncCommittee
	sszSlice23 := buf[2712005:2736629] // c.NextSyncCommittee
	sszSlice24 := buf[2736629:2736661] // c.LatestBlockHash
	sszSlice25 := buf[2736661:2736669] // c.NextWithdrawalIndex
	sszSlice26 := buf[2736669:2736677] // c.NextWithdrawalValidatorIndex
	sszSlice28 := buf[2736681:2736689] // c.DepositRequestsStartIndex
	sszSlice29 := buf[2736689:2736697] // c.DepositBalanceToConsume
	sszSlice30 := buf[2736697:2736705] // c.ExitBalanceToConsume
	sszSlice31 := buf[2736705:2736713] // c.EarliestExitEpoch
	sszSlice32 := buf[2736713:2736721] // c.ConsolidationBalanceToConsume
	sszSlice33 := buf[2736721:2736729] // c.EarliestConsolidationEpoch
	sszSlice37 := buf[2736741:2737253] // c.ProposerLookahead
	sszSlice39 := buf[2737257:2737265] // c.NextWithdrawalBuilderIndex
	sszSlice40 := buf[2737265:2738289] // c.ExecutionPayloadAvailability
	sszSlice41 := buf[2738289:2741617] // c.BuilderPendingPayments
	sszSlice45 := buf[2741629:3134845] // c.PTCWindow

	sszVarOffset7 := ssz.ReadOffset(buf[524464:524468]) // c.HistoricalRoots
	if sszVarOffset7 != 3134845 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset7 > size {
		return ssz.ErrOffset
	}
	sszVarOffset9 := ssz.ReadOffset(buf[524540:524544]) // c.ETH1DataVotes
	if sszVarOffset9 > size || sszVarOffset9 < sszVarOffset7 {
		return ssz.ErrOffset
	}
	sszVarOffset11 := ssz.ReadOffset(buf[524552:524556]) // c.Validators
	if sszVarOffset11 > size || sszVarOffset11 < sszVarOffset9 {
		return ssz.ErrOffset
	}
	sszVarOffset12 := ssz.ReadOffset(buf[524556:524560]) // c.Balances
	if sszVarOffset12 > size || sszVarOffset12 < sszVarOffset11 {
		return ssz.ErrOffset
	}
	sszVarOffset15 := ssz.ReadOffset(buf[2687248:2687252]) // c.PreviousEpochParticipation
	if sszVarOffset15 > size || sszVarOffset15 < sszVarOffset12 {
		return ssz.ErrOffset
	}
	sszVarOffset16 := ssz.ReadOffset(buf[2687252:2687256]) // c.CurrentEpochParticipation
	if sszVarOffset16 > size || sszVarOffset16 < sszVarOffset15 {
		return ssz.ErrOffset
	}
	sszVarOffset21 := ssz.ReadOffset(buf[2687377:2687381]) // c.InactivityScores
	if sszVarOffset21 > size || sszVarOffset21 < sszVarOffset16 {
		return ssz.ErrOffset
	}
	sszVarOffset27 := ssz.ReadOffset(buf[2736677:2736681]) // c.HistoricalSummaries
	if sszVarOffset27 > size || sszVarOffset27 < sszVarOffset21 {
		return ssz.ErrOffset
	}
	sszVarOffset34 := ssz.ReadOffset(buf[2736729:2736733]) // c.PendingDeposits
	if sszVarOffset34 > size || sszVarOffset34 < sszVarOffset27 {
		return ssz.ErrOffset
	}
	sszVarOffset35 := ssz.ReadOffset(buf[2736733:2736737]) // c.PendingPartialWithdrawals
	if sszVarOffset35 > size || sszVarOffset35 < sszVarOffset34 {
		return ssz.ErrOffset
	}
	sszVarOffset36 := ssz.ReadOffset(buf[2736737:2736741]) // c.PendingConsolidations
	if sszVarOffset36 > size || sszVarOffset36 < sszVarOffset35 {
		return ssz.ErrOffset
	}
	sszVarOffset38 := ssz.ReadOffset(buf[2737253:2737257]) // c.Builders
	if sszVarOffset38 > size || sszVarOffset38 < sszVarOffset36 {
		return ssz.ErrOffset
	}
	sszVarOffset42 := ssz.ReadOffset(buf[2741617:2741621]) // c.BuilderPendingWithdrawals
	if sszVarOffset42 > size || sszVarOffset42 < sszVarOffset38 {
		return ssz.ErrOffset
	}
	sszVarOffset43 := ssz.ReadOffset(buf[2741621:2741625]) // c.LatestExecutionPayloadBid
	if sszVarOffset43 > size || sszVarOffset43 < sszVarOffset42 {
		return ssz.ErrOffset
	}
	sszVarOffset44 := ssz.ReadOffset(buf[2741625:2741629]) // c.PayloadExpectedWithdrawals
	if sszVarOffset44 > size || sszVarOffset44 < sszVarOffset43 {
		return ssz.ErrOffset
	}
	sszSlice7 := buf[sszVarOffset7:sszVarOffset9]    // c.HistoricalRoots
	sszSlice9 := buf[sszVarOffset9:sszVarOffset11]   // c.ETH1DataVotes
	sszSlice11 := buf[sszVarOffset11:sszVarOffset12] // c.Validators
	sszSlice12 := buf[sszVarOffset12:sszVarOffset15] // c.Balances
	sszSlice15 := buf[sszVarOffset15:sszVarOffset16] // c.PreviousEpochParticipation
	sszSlice16 := buf[sszVarOffset16:sszVarOffset21] // c.CurrentEpochParticipation
	sszSlice21 := buf[sszVarOffset21:sszVarOffset27] // c.InactivityScores
	sszSlice27 := buf[sszVarOffset27:sszVarOffset34] // c.HistoricalSummaries
	sszSlice34 := buf[sszVarOffset34:sszVarOffset35] // c.PendingDeposits
	sszSlice35 := buf[sszVarOffset35:sszVarOffset36] // c.PendingPartialWithdrawals
	sszSlice36 := buf[sszVarOffset36:sszVarOffset38] // c.PendingConsolidations
	sszSlice38 := buf[sszVarOffset38:sszVarOffset42] // c.Builders
	sszSlice42 := buf[sszVarOffset42:sszVarOffset43] // c.BuilderPendingWithdrawals
	sszSlice43 := buf[sszVarOffset43:sszVarOffset44] // c.LatestExecutionPayloadBid
	sszSlice44 := buf[sszVarOffset44:]               // c.PayloadExpectedWithdrawals

	// Field 0: GenesisTime
	c.GenesisTime = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: GenesisValidatorsRoot
	copy(c.GenesisValidatorsRoot[:], sszSlice1)

	// Field 2: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Fork
	c.Fork = new(Fork)
	if err = c.Fork.UnmarshalSSZ(sszSlice3); err != nil {
		return fmt.Errorf("Fork: %w", err)
	}

	// Field 4: LatestBlockHeader
	c.LatestBlockHeader = new(BeaconBlockHeader)
	if err = c.LatestBlockHeader.UnmarshalSSZ(sszSlice4); err != nil {
		return fmt.Errorf("LatestBlockHeader: %w", err)
	}

	// Field 5: BlockRoots
	{
		c.BlockRoots = make([][32]byte, 8192)
		for i := 0; i < 8192; i++ {
			var tmp [32]byte

			tmpSlice := sszSlice5[i*32 : (1+i)*32]
			copy(tmp[:], tmpSlice)
			c.BlockRoots[i] = tmp
		}
	}

	// Field 6: StateRoots
	{
		c.StateRoots = make([][32]byte, 8192)
		for i := 0; i < 8192; i++ {
			var tmp [32]byte

			tmpSlice := sszSlice6[i*32 : (1+i)*32]
			copy(tmp[:], tmpSlice)
			c.StateRoots[i] = tmp
		}
	}

	// Field 7: HistoricalRoots
	{
		if len(sszSlice7)%32 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalRoots length is %d, which is not a multiple of 32: %w", len(sszSlice7), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice7) / 32
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalRoots has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalRoots = make([][32]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp [32]byte

			tmpSlice := sszSlice7[i*32 : (1+i)*32]
			copy(tmp[:], tmpSlice)
			c.HistoricalRoots[i] = tmp
		}
	}

	// Field 8: ETH1Data
	c.ETH1Data = new(ETH1Data)
	if err = c.ETH1Data.UnmarshalSSZ(sszSlice8); err != nil {
		return fmt.Errorf("ETH1Data: %w", err)
	}

	// Field 9: ETH1DataVotes
	{
		if len(sszSlice9)%72 != 0 {
			return fmt.Errorf("misaligned bytes: c.ETH1DataVotes length is %d, which is not a multiple of 72: %w", len(sszSlice9), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice9) / 72
		if numElem > 2048 {
			return fmt.Errorf("ssz-max exceeded: c.ETH1DataVotes has %d elements, ssz-max is 2048: %w", numElem, ssz.ErrListTooBig)
		}
		c.ETH1DataVotes = make([]*ETH1Data, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ETH1Data
			tmp = new(ETH1Data)
			tmpSlice := sszSlice9[i*72 : (1+i)*72]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("ETH1DataVotes: %w", err)
			}
			c.ETH1DataVotes[i] = tmp
		}
	}

	// Field 10: ETH1DepositIndex
	c.ETH1DepositIndex = binary.LittleEndian.Uint64(sszSlice10)

	// Field 11: Validators
	{
		if len(sszSlice11)%121 != 0 {
			return fmt.Errorf("misaligned bytes: c.Validators length is %d, which is not a multiple of 121: %w", len(sszSlice11), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice11) / 121
		c.Validators = make([]*Validator, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *Validator
			tmp = new(Validator)
			tmpSlice := sszSlice11[i*121 : (1+i)*121]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
			c.Validators[i] = tmp
		}
	}

	// Field 12: Balances
	{
		if len(sszSlice12)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.Balances length is %d, which is not a multiple of 8: %w", len(sszSlice12), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice12) / 8
		c.Balances = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice12[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Balances[i] = tmp
		}
	}

	// Field 13: RANDAOMixes
	{
		c.RANDAOMixes = make([][32]byte, 65536)
		for i := 0; i < 65536; i++ {
			var tmp [32]byte

			tmpSlice := sszSlice13[i*32 : (1+i)*32]
			copy(tmp[:], tmpSlice)
			c.RANDAOMixes[i] = tmp
		}
	}

	// Field 14: Slashings
	{
		c.Slashings = make([]uint64, 8192)
		for i := 0; i < 8192; i++ {
			var tmp uint64

			tmpSlice := sszSlice14[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.Slashings[i] = tmp
		}
	}

	// Field 15: PreviousEpochParticipation
	c.PreviousEpochParticipation = append([]byte{}, sszSlice15...)

	// Field 16: CurrentEpochParticipation
	c.CurrentEpochParticipation = append([]byte{}, sszSlice16...)

	// Field 17: JustificationBits
	c.JustificationBits = make([]byte, 0, 1)
	c.JustificationBits = append(c.JustificationBits, go_bitfield.Bitvector4(sszSlice17)...)

	// Field 18: PreviousJustifiedCheckpoint
	c.PreviousJustifiedCheckpoint = new(Checkpoint)
	if err = c.PreviousJustifiedCheckpoint.UnmarshalSSZ(sszSlice18); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}

	// Field 19: CurrentJustifiedCheckpoint
	c.CurrentJustifiedCheckpoint = new(Checkpoint)
	if err = c.CurrentJustifiedCheckpoint.UnmarshalSSZ(sszSlice19); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}

	// Field 20: FinalizedCheckpoint
	c.FinalizedCheckpoint = new(Checkpoint)
	if err = c.FinalizedCheckpoint.UnmarshalSSZ(sszSlice20); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}

	// Field 21: InactivityScores
	{
		if len(sszSlice21)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.InactivityScores length is %d, which is not a multiple of 8: %w", len(sszSlice21), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice21) / 8
		c.InactivityScores = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice21[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.InactivityScores[i] = tmp
		}
	}

	// Field 22: CurrentSyncCommittee
	c.CurrentSyncCommittee = new(AltairSyncCommittee)
	if err = c.CurrentSyncCommittee.UnmarshalSSZ(sszSlice22); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}

	// Field 23: NextSyncCommittee
	c.NextSyncCommittee = new(AltairSyncCommittee)
	if err = c.NextSyncCommittee.UnmarshalSSZ(sszSlice23); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}

	// Field 24: LatestBlockHash
	copy(c.LatestBlockHash[:], sszSlice24)

	// Field 25: NextWithdrawalIndex
	c.NextWithdrawalIndex = binary.LittleEndian.Uint64(sszSlice25)

	// Field 26: NextWithdrawalValidatorIndex
	c.NextWithdrawalValidatorIndex = binary.LittleEndian.Uint64(sszSlice26)

	// Field 27: HistoricalSummaries
	{
		if len(sszSlice27)%64 != 0 {
			return fmt.Errorf("misaligned bytes: c.HistoricalSummaries length is %d, which is not a multiple of 64: %w", len(sszSlice27), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice27) / 64
		if numElem > 16777216 {
			return fmt.Errorf("ssz-max exceeded: c.HistoricalSummaries has %d elements, ssz-max is 16777216: %w", numElem, ssz.ErrListTooBig)
		}
		c.HistoricalSummaries = make([]*CapellaHistoricalSummary, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *CapellaHistoricalSummary
			tmp = new(CapellaHistoricalSummary)
			tmpSlice := sszSlice27[i*64 : (1+i)*64]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
			c.HistoricalSummaries[i] = tmp
		}
	}

	// Field 28: DepositRequestsStartIndex
	c.DepositRequestsStartIndex = binary.LittleEndian.Uint64(sszSlice28)

	// Field 29: DepositBalanceToConsume
	c.DepositBalanceToConsume = binary.LittleEndian.Uint64(sszSlice29)

	// Field 30: ExitBalanceToConsume
	c.ExitBalanceToConsume = binary.LittleEndian.Uint64(sszSlice30)

	// Field 31: EarliestExitEpoch
	c.EarliestExitEpoch = binary.LittleEndian.Uint64(sszSlice31)

	// Field 32: ConsolidationBalanceToConsume
	c.ConsolidationBalanceToConsume = binary.LittleEndian.Uint64(sszSlice32)

	// Field 33: EarliestConsolidationEpoch
	c.EarliestConsolidationEpoch = binary.LittleEndian.Uint64(sszSlice33)

	// Field 34: PendingDeposits
	{
		if len(sszSlice34)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingDeposits length is %d, which is not a multiple of 192: %w", len(sszSlice34), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice34) / 192
		c.PendingDeposits = make([]*ElectraPendingDeposit, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraPendingDeposit
			tmp = new(ElectraPendingDeposit)
			tmpSlice := sszSlice34[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
			c.PendingDeposits[i] = tmp
		}
	}

	// Field 35: PendingPartialWithdrawals
	{
		if len(sszSlice35)%24 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingPartialWithdrawals length is %d, which is not a multiple of 24: %w", len(sszSlice35), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice35) / 24
		c.PendingPartialWithdrawals = make([]*ElectraPendingPartialWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraPendingPartialWithdrawal
			tmp = new(ElectraPendingPartialWithdrawal)
			tmpSlice := sszSlice35[i*24 : (1+i)*24]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
			c.PendingPartialWithdrawals[i] = tmp
		}
	}

	// Field 36: PendingConsolidations
	{
		if len(sszSlice36)%16 != 0 {
			return fmt.Errorf("misaligned bytes: c.PendingConsolidations length is %d, which is not a multiple of 16: %w", len(sszSlice36), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice36) / 16
		c.PendingConsolidations = make([]*ElectraPendingConsolidation, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraPendingConsolidation
			tmp = new(ElectraPendingConsolidation)
			tmpSlice := sszSlice36[i*16 : (1+i)*16]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
			c.PendingConsolidations[i] = tmp
		}
	}

	// Field 37: ProposerLookahead
	{
		c.ProposerLookahead = make([]uint64, 64)
		for i := 0; i < 64; i++ {
			var tmp uint64

			tmpSlice := sszSlice37[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.ProposerLookahead[i] = tmp
		}
	}

	// Field 38: Builders
	{
		if len(sszSlice38)%93 != 0 {
			return fmt.Errorf("misaligned bytes: c.Builders length is %d, which is not a multiple of 93: %w", len(sszSlice38), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice38) / 93
		c.Builders = make([]*GloasBuilder, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *GloasBuilder
			tmp = new(GloasBuilder)
			tmpSlice := sszSlice38[i*93 : (1+i)*93]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
			c.Builders[i] = tmp
		}
	}

	// Field 39: NextWithdrawalBuilderIndex
	c.NextWithdrawalBuilderIndex = binary.LittleEndian.Uint64(sszSlice39)

	// Field 40: ExecutionPayloadAvailability
	c.ExecutionPayloadAvailability = make([]byte, 0, 1024)
	c.ExecutionPayloadAvailability = append(c.ExecutionPayloadAvailability, sszSlice40...)

	// Field 41: BuilderPendingPayments
	{
		c.BuilderPendingPayments = make([]*GloasBuilderPendingPayment, 64)
		for i := 0; i < 64; i++ {
			var tmp *GloasBuilderPendingPayment
			tmp = new(GloasBuilderPendingPayment)
			tmpSlice := sszSlice41[i*52 : (1+i)*52]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
			c.BuilderPendingPayments[i] = tmp
		}
	}

	// Field 42: BuilderPendingWithdrawals
	{
		if len(sszSlice42)%36 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderPendingWithdrawals length is %d, which is not a multiple of 36: %w", len(sszSlice42), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice42) / 36
		c.BuilderPendingWithdrawals = make([]*GloasBuilderPendingWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *GloasBuilderPendingWithdrawal
			tmp = new(GloasBuilderPendingWithdrawal)
			tmpSlice := sszSlice42[i*36 : (1+i)*36]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
			c.BuilderPendingWithdrawals[i] = tmp
		}
	}

	// Field 43: LatestExecutionPayloadBid
	c.LatestExecutionPayloadBid = new(GloasExecutionPayloadBid)
	if err = c.LatestExecutionPayloadBid.UnmarshalSSZ(sszSlice43); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}

	// Field 44: PayloadExpectedWithdrawals
	{
		if len(sszSlice44)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.PayloadExpectedWithdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice44), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice44) / 44
		c.PayloadExpectedWithdrawals = make([]*CapellaWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *CapellaWithdrawal
			tmp = new(CapellaWithdrawal)
			tmpSlice := sszSlice44[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
			c.PayloadExpectedWithdrawals[i] = tmp
		}
	}

	// Field 45: PTCWindow
	{
		c.PTCWindow = make([][]uint64, 96)
		for i := 0; i < 96; i++ {
			var tmp []uint64

			tmpSlice := sszSlice45[i*4096 : (1+i)*4096]
			{
				tmp = make([]uint64, 512)
				for i := 0; i < 512; i++ {
					var tmp2 uint64

					tmpSlice2 := tmpSlice[i*8 : (1+i)*8]
					tmp2 = binary.LittleEndian.Uint64(tmpSlice2)
					tmp[i] = tmp2
				}
			}
			c.PTCWindow[i] = tmp
		}
	}
	return err
}

func (c *GloasBeaconState) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasBeaconState) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasBeaconState = []byte{0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b11111111, 0b00111111}

func (c *GloasBeaconState) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBeaconState) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: GenesisTime
	hh.PutUint64(c.GenesisTime)
	// Field 1: GenesisValidatorsRoot
	if len(c.GenesisValidatorsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.GenesisValidatorsRoot[:])
	// Field 2: Slot
	hh.PutUint64(c.Slot)
	// Field 3: Fork
	if err := c.Fork.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Fork: %w", err)
	}
	// Field 4: LatestBlockHeader
	if err := c.LatestBlockHeader.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestBlockHeader: %w", err)
	}
	// Field 5: BlockRoots
	{
		if len(c.BlockRoots) != 8192 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.BlockRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o[:])
		}
		hh.Merkleize(subIndx)
	}
	// Field 6: StateRoots
	{
		if len(c.StateRoots) != 8192 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.StateRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o[:])
		}
		hh.Merkleize(subIndx)
	}
	// Field 7: HistoricalRoots
	{
		if len(c.HistoricalRoots) > 16777216 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.HistoricalRoots {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o[:])
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.HistoricalRoots)), 16777216)
	}
	// Field 8: ETH1Data
	if err := c.ETH1Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ETH1Data: %w", err)
	}
	// Field 9: ETH1DataVotes
	{
		if len(c.ETH1DataVotes) > 2048 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.ETH1DataVotes {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("ETH1DataVotes: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.ETH1DataVotes)), 2048)
	}
	// Field 10: ETH1DepositIndex
	hh.PutUint64(c.ETH1DepositIndex)
	// Field 11: Validators
	{
		subIndx := hh.Index()
		for _, o := range c.Validators {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Validators: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Validators)))
	}
	// Field 12: Balances
	{
		subIndx := hh.Index()
		for _, o := range c.Balances {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Balances)))
	}
	// Field 13: RANDAOMixes
	{
		if len(c.RANDAOMixes) != 65536 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.RANDAOMixes {
			if len(o) != 32 {
				return ssz.ErrBytesLength
			}
			hh.Append(o[:])
		}
		hh.Merkleize(subIndx)
	}
	// Field 14: Slashings
	{
		if len(c.Slashings) != 8192 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.Slashings {
			hh.AppendUint64(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 15: PreviousEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.PreviousEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PreviousEpochParticipation)))
	}
	// Field 16: CurrentEpochParticipation
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.CurrentEpochParticipation)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.CurrentEpochParticipation)))
	}
	// Field 17: JustificationBits
	if len([]byte(c.JustificationBits)) != 1 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.JustificationBits))
	// Field 18: PreviousJustifiedCheckpoint
	if err := c.PreviousJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("PreviousJustifiedCheckpoint: %w", err)
	}
	// Field 19: CurrentJustifiedCheckpoint
	if err := c.CurrentJustifiedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentJustifiedCheckpoint: %w", err)
	}
	// Field 20: FinalizedCheckpoint
	if err := c.FinalizedCheckpoint.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("FinalizedCheckpoint: %w", err)
	}
	// Field 21: InactivityScores
	{
		subIndx := hh.Index()
		for _, o := range c.InactivityScores {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.InactivityScores)))
	}
	// Field 22: CurrentSyncCommittee
	if err := c.CurrentSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("CurrentSyncCommittee: %w", err)
	}
	// Field 23: NextSyncCommittee
	if err := c.NextSyncCommittee.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("NextSyncCommittee: %w", err)
	}
	// Field 24: LatestBlockHash
	if len(c.LatestBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LatestBlockHash[:])
	// Field 25: NextWithdrawalIndex
	hh.PutUint64(c.NextWithdrawalIndex)
	// Field 26: NextWithdrawalValidatorIndex
	hh.PutUint64(c.NextWithdrawalValidatorIndex)
	// Field 27: HistoricalSummaries
	{
		if len(c.HistoricalSummaries) > 16777216 {
			return ssz.ErrListTooBig
		}
		subIndx := hh.Index()
		for _, o := range c.HistoricalSummaries {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("HistoricalSummaries: %w", err)
			}
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(c.HistoricalSummaries)), 16777216)
	}
	// Field 28: DepositRequestsStartIndex
	hh.PutUint64(c.DepositRequestsStartIndex)
	// Field 29: DepositBalanceToConsume
	hh.PutUint64(c.DepositBalanceToConsume)
	// Field 30: ExitBalanceToConsume
	hh.PutUint64(c.ExitBalanceToConsume)
	// Field 31: EarliestExitEpoch
	hh.PutUint64(c.EarliestExitEpoch)
	// Field 32: ConsolidationBalanceToConsume
	hh.PutUint64(c.ConsolidationBalanceToConsume)
	// Field 33: EarliestConsolidationEpoch
	hh.PutUint64(c.EarliestConsolidationEpoch)
	// Field 34: PendingDeposits
	{
		subIndx := hh.Index()
		for _, o := range c.PendingDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingDeposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingDeposits)))
	}
	// Field 35: PendingPartialWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PendingPartialWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingPartialWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingPartialWithdrawals)))
	}
	// Field 36: PendingConsolidations
	{
		subIndx := hh.Index()
		for _, o := range c.PendingConsolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PendingConsolidations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PendingConsolidations)))
	}
	// Field 37: ProposerLookahead
	{
		if len(c.ProposerLookahead) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.ProposerLookahead {
			hh.AppendUint64(o)
		}
		hh.Merkleize(subIndx)
	}
	// Field 38: Builders
	{
		subIndx := hh.Index()
		for _, o := range c.Builders {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Builders: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Builders)))
	}
	// Field 39: NextWithdrawalBuilderIndex
	hh.PutUint64(c.NextWithdrawalBuilderIndex)
	// Field 40: ExecutionPayloadAvailability
	if len(c.ExecutionPayloadAvailability) != 1024 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionPayloadAvailability)
	// Field 41: BuilderPendingPayments
	{
		if len(c.BuilderPendingPayments) != 64 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingPayments {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingPayments: %w", err)
			}
		}
		hh.Merkleize(subIndx)
	}
	// Field 42: BuilderPendingWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderPendingWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderPendingWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderPendingWithdrawals)))
	}
	// Field 43: LatestExecutionPayloadBid
	if err := c.LatestExecutionPayloadBid.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("LatestExecutionPayloadBid: %w", err)
	}
	// Field 44: PayloadExpectedWithdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.PayloadExpectedWithdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("PayloadExpectedWithdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.PayloadExpectedWithdrawals)))
	}
	// Field 45: PTCWindow
	{
		if len(c.PTCWindow) != 96 {
			return ssz.ErrVectorLength
		}
		subIndx := hh.Index()
		for _, o := range c.PTCWindow {
			{
				if len(o) != 512 {
					return ssz.ErrVectorLength
				}
				subIndx := hh.Index()
				for _, oo := range o {
					hh.AppendUint64(oo)
				}
				hh.Merkleize(subIndx)
			}
		}
		hh.Merkleize(subIndx)
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasBeaconState)
	return nil
}

func (c *GloasBuilder) SizeSSZ() int {
	size := 93

	return size
}

func (c *GloasBuilder) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBuilder) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PublicKey[:]...)

	// Field 1: Version
	dst = append(dst, c.Version)

	// Field 2: ExecutionAddress
	if len(c.ExecutionAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionAddress[:]...)

	// Field 3: Balance
	dst = binary.LittleEndian.AppendUint64(dst, c.Balance)

	// Field 4: DepositEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.DepositEpoch)

	// Field 5: WithdrawableEpoch
	dst = binary.LittleEndian.AppendUint64(dst, c.WithdrawableEpoch)

	return dst, err
}

func (c *GloasBuilder) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 93 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]  // c.PublicKey
	sszSlice1 := buf[48:49] // c.Version
	sszSlice2 := buf[49:69] // c.ExecutionAddress
	sszSlice3 := buf[69:77] // c.Balance
	sszSlice4 := buf[77:85] // c.DepositEpoch
	sszSlice5 := buf[85:93] // c.WithdrawableEpoch

	// Field 0: PublicKey
	copy(c.PublicKey[:], sszSlice0)

	// Field 1: Version
	c.Version = sszSlice1[0]

	// Field 2: ExecutionAddress
	copy(c.ExecutionAddress[:], sszSlice2)

	// Field 3: Balance
	c.Balance = binary.LittleEndian.Uint64(sszSlice3)

	// Field 4: DepositEpoch
	c.DepositEpoch = binary.LittleEndian.Uint64(sszSlice4)

	// Field 5: WithdrawableEpoch
	c.WithdrawableEpoch = binary.LittleEndian.Uint64(sszSlice5)
	return err
}

func (c *GloasBuilder) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBuilder) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: PublicKey
	if len(c.PublicKey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PublicKey[:])
	// Field 1: Version
	hh.PutUint8(c.Version)
	// Field 2: ExecutionAddress
	if len(c.ExecutionAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionAddress[:])
	// Field 3: Balance
	hh.PutUint64(c.Balance)
	// Field 4: DepositEpoch
	hh.PutUint64(c.DepositEpoch)
	// Field 5: WithdrawableEpoch
	hh.PutUint64(c.WithdrawableEpoch)
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBuilderDepositRequest) SizeSSZ() int {
	size := 184

	return size
}

func (c *GloasBuilderDepositRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBuilderDepositRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey[:]...)

	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.WithdrawalCredentials...)

	// Field 2: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 3: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *GloasBuilderDepositRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 184 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:48]   // c.Pubkey
	sszSlice1 := buf[48:80]  // c.WithdrawalCredentials
	sszSlice2 := buf[80:88]  // c.Amount
	sszSlice3 := buf[88:184] // c.Signature

	// Field 0: Pubkey
	copy(c.Pubkey[:], sszSlice0)

	// Field 1: WithdrawalCredentials
	c.WithdrawalCredentials = make([]byte, 0, 32)
	c.WithdrawalCredentials = append(c.WithdrawalCredentials, sszSlice1...)

	// Field 2: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: Signature
	copy(c.Signature[:], sszSlice3)
	return err
}

func (c *GloasBuilderDepositRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBuilderDepositRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey[:])
	// Field 1: WithdrawalCredentials
	if len(c.WithdrawalCredentials) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.WithdrawalCredentials)
	// Field 2: Amount
	hh.PutUint64(c.Amount)
	// Field 3: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBuilderExitRequest) SizeSSZ() int {
	size := 68

	return size
}

func (c *GloasBuilderExitRequest) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBuilderExitRequest) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.SourceAddress[:]...)

	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Pubkey[:]...)

	return dst, err
}

func (c *GloasBuilderExitRequest) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 68 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.SourceAddress
	sszSlice1 := buf[20:68] // c.Pubkey

	// Field 0: SourceAddress
	copy(c.SourceAddress[:], sszSlice0)

	// Field 1: Pubkey
	copy(c.Pubkey[:], sszSlice1)
	return err
}

func (c *GloasBuilderExitRequest) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBuilderExitRequest) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: SourceAddress
	if len(c.SourceAddress) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.SourceAddress[:])
	// Field 1: Pubkey
	if len(c.Pubkey) != 48 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Pubkey[:])
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBuilderPendingPayment) SizeSSZ() int {
	size := 52

	return size
}

func (c *GloasBuilderPendingPayment) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBuilderPendingPayment) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: Weight
	dst = binary.LittleEndian.AppendUint64(dst, c.Weight)

	// Field 1: Withdrawal
	if c.Withdrawal == nil {
		c.Withdrawal = new(GloasBuilderPendingWithdrawal)
	}
	if dst, err = c.Withdrawal.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Withdrawal: %w", err)
	}

	// Field 2: ProposerIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.ProposerIndex)

	return dst, err
}

func (c *GloasBuilderPendingPayment) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 52 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:8]   // c.Weight
	sszSlice1 := buf[8:44]  // c.Withdrawal
	sszSlice2 := buf[44:52] // c.ProposerIndex

	// Field 0: Weight
	c.Weight = binary.LittleEndian.Uint64(sszSlice0)

	// Field 1: Withdrawal
	c.Withdrawal = new(GloasBuilderPendingWithdrawal)
	if err = c.Withdrawal.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}

	// Field 2: ProposerIndex
	c.ProposerIndex = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *GloasBuilderPendingPayment) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBuilderPendingPayment) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Weight
	hh.PutUint64(c.Weight)
	// Field 1: Withdrawal
	if err := c.Withdrawal.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Withdrawal: %w", err)
	}
	// Field 2: ProposerIndex
	hh.PutUint64(c.ProposerIndex)
	hh.Merkleize(indx)
	return nil
}

func (c *GloasBuilderPendingWithdrawal) SizeSSZ() int {
	size := 36

	return size
}

func (c *GloasBuilderPendingWithdrawal) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasBuilderPendingWithdrawal) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient[:]...)

	// Field 1: Amount
	dst = binary.LittleEndian.AppendUint64(dst, c.Amount)

	// Field 2: BuilderIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.BuilderIndex)

	return dst, err
}

func (c *GloasBuilderPendingWithdrawal) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 36 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:20]  // c.FeeRecipient
	sszSlice1 := buf[20:28] // c.Amount
	sszSlice2 := buf[28:36] // c.BuilderIndex

	// Field 0: FeeRecipient
	copy(c.FeeRecipient[:], sszSlice0)

	// Field 1: Amount
	c.Amount = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: BuilderIndex
	c.BuilderIndex = binary.LittleEndian.Uint64(sszSlice2)
	return err
}

func (c *GloasBuilderPendingWithdrawal) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasBuilderPendingWithdrawal) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient[:])
	// Field 1: Amount
	hh.PutUint64(c.Amount)
	// Field 2: BuilderIndex
	hh.PutUint64(c.BuilderIndex)
	hh.Merkleize(indx)
	return nil
}

func (c *GloasExecutionPayload) SizeSSZ() int {
	size := 540
	size += len(c.ExtraData)
	for _, o := range c.Transactions {
		size += 4
		size += len(o)
	}
	size += len(c.Withdrawals) * 44
	size += len(c.BlockAccessList)
	return size
}

func (c *GloasExecutionPayload) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasExecutionPayload) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 540

	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentHash[:]...)

	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient[:]...)

	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.StateRoot[:]...)

	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ReceiptsRoot[:]...)

	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.LogsBloom[:]...)

	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao[:]...)

	// Field 6: BlockNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.BlockNumber)

	// Field 7: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 8: GasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.GasUsed)

	// Field 9: Timestamp
	dst = binary.LittleEndian.AppendUint64(dst, c.Timestamp)

	// Field 10: ExtraData
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.ExtraData)

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BaseFeePerGas[:]...)

	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash[:]...)

	// Field 13: Transactions
	dst = ssz.WriteOffset(dst, offset)
	for _, o := range c.Transactions {
		offset += 4
		offset += len(o)
	}

	// Field 14: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 44

	// Field 15: BlobGasUsed
	dst = binary.LittleEndian.AppendUint64(dst, c.BlobGasUsed)

	// Field 16: ExcessBlobGas
	dst = binary.LittleEndian.AppendUint64(dst, c.ExcessBlobGas)

	// Field 17: BlockAccessList
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlockAccessList)

	// Field 18: SlotNumber
	dst = binary.LittleEndian.AppendUint64(dst, c.SlotNumber)

	// Field 10: ExtraData
	if len(c.ExtraData) > 32 {
		return nil, ssz.ErrListTooBig
	}
	dst = append(dst, c.ExtraData...)

	// Field 13: Transactions
	{
		offset = 4 * len(c.Transactions)
		for _, o := range c.Transactions {
			dst = ssz.WriteOffset(dst, offset)
			offset += len(o)
		}
	}
	for _, o := range c.Transactions {
		dst = append(dst, o...)
	}

	// Field 14: Withdrawals
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}

	// Field 17: BlockAccessList
	dst = append(dst, c.BlockAccessList...)
	return dst, err
}

func (c *GloasExecutionPayload) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 540 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentHash
	sszSlice1 := buf[32:52]    // c.FeeRecipient
	sszSlice2 := buf[52:84]    // c.StateRoot
	sszSlice3 := buf[84:116]   // c.ReceiptsRoot
	sszSlice4 := buf[116:372]  // c.LogsBloom
	sszSlice5 := buf[372:404]  // c.PrevRandao
	sszSlice6 := buf[404:412]  // c.BlockNumber
	sszSlice7 := buf[412:420]  // c.GasLimit
	sszSlice8 := buf[420:428]  // c.GasUsed
	sszSlice9 := buf[428:436]  // c.Timestamp
	sszSlice11 := buf[440:472] // c.BaseFeePerGas
	sszSlice12 := buf[472:504] // c.BlockHash
	sszSlice15 := buf[512:520] // c.BlobGasUsed
	sszSlice16 := buf[520:528] // c.ExcessBlobGas
	sszSlice18 := buf[532:540] // c.SlotNumber

	sszVarOffset10 := ssz.ReadOffset(buf[436:440]) // c.ExtraData
	if sszVarOffset10 != 540 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszVarOffset13 := ssz.ReadOffset(buf[504:508]) // c.Transactions
	if sszVarOffset13 > size || sszVarOffset13 < sszVarOffset10 {
		return ssz.ErrOffset
	}
	sszVarOffset14 := ssz.ReadOffset(buf[508:512]) // c.Withdrawals
	if sszVarOffset14 > size || sszVarOffset14 < sszVarOffset13 {
		return ssz.ErrOffset
	}
	sszVarOffset17 := ssz.ReadOffset(buf[528:532]) // c.BlockAccessList
	if sszVarOffset17 > size || sszVarOffset17 < sszVarOffset14 {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:sszVarOffset13] // c.ExtraData
	sszSlice13 := buf[sszVarOffset13:sszVarOffset14] // c.Transactions
	sszSlice14 := buf[sszVarOffset14:sszVarOffset17] // c.Withdrawals
	sszSlice17 := buf[sszVarOffset17:]               // c.BlockAccessList

	// Field 0: ParentHash
	copy(c.ParentHash[:], sszSlice0)

	// Field 1: FeeRecipient
	copy(c.FeeRecipient[:], sszSlice1)

	// Field 2: StateRoot
	copy(c.StateRoot[:], sszSlice2)

	// Field 3: ReceiptsRoot
	copy(c.ReceiptsRoot[:], sszSlice3)

	// Field 4: LogsBloom
	copy(c.LogsBloom[:], sszSlice4)

	// Field 5: PrevRandao
	copy(c.PrevRandao[:], sszSlice5)

	// Field 6: BlockNumber
	c.BlockNumber = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: GasUsed
	c.GasUsed = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: Timestamp
	c.Timestamp = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: ExtraData
	c.ExtraData = append([]byte{}, sszSlice10...)

	// Field 11: BaseFeePerGas
	copy(c.BaseFeePerGas[:], sszSlice11)

	// Field 12: BlockHash
	copy(c.BlockHash[:], sszSlice12)

	// Field 13: Transactions
	{
		// empty lists are zero length, so make sure there is room for an offset
		// before attempting to unmarshal it
		if len(sszSlice13) > 3 {
			startOffset := ssz.ReadOffset(sszSlice13[0:4])
			if startOffset == 0 {
				return fmt.Errorf("encountered invalid offset of 0 when decoding c.Transactions")
			}
			if startOffset%4 != 0 {
				return fmt.Errorf("misaligned list bytes: when decoding c.Transactions, end-of-list offset is %d, which is not a multiple of 4 (offset size)", startOffset)
			}
			listLen := startOffset / 4
			totalVarBytes := uint64(len(sszSlice13))
			if totalVarBytes < startOffset {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, listLen)
			var tmpSlice []byte
			for i := uint64(0); i < listLen; i++ {
				var tmp []byte

				endOffset := totalVarBytes
				if i+1 != listLen {
					endOffset = ssz.ReadOffset(sszSlice13[(i+1)*4 : (i+2)*4])
					if totalVarBytes < endOffset {
						return fmt.Errorf("offset %d points past the end of buffer when decoding c.Transactions", endOffset)
					}
				}
				if endOffset < startOffset {
					return fmt.Errorf("offset %d is not greater than start offset %d when decoding c.Transactions", endOffset, startOffset)
				}
				tmpSlice = sszSlice13[startOffset:endOffset]
				tmp = append([]byte{}, tmpSlice...)
				c.Transactions[i] = tmp
				startOffset = endOffset
			}
		} else {
			if len(sszSlice13) > 0 {
				return fmt.Errorf("list bytes too short to contain an offset when decoding c.Transactions")
			}
			c.Transactions = make([][]byte, 0)
		}
	}

	// Field 14: Withdrawals
	{
		if len(sszSlice14)%44 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 44: %w", len(sszSlice14), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice14) / 44
		c.Withdrawals = make([]*CapellaWithdrawal, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *CapellaWithdrawal
			tmp = new(CapellaWithdrawal)
			tmpSlice := sszSlice14[i*44 : (1+i)*44]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 15: BlobGasUsed
	c.BlobGasUsed = binary.LittleEndian.Uint64(sszSlice15)

	// Field 16: ExcessBlobGas
	c.ExcessBlobGas = binary.LittleEndian.Uint64(sszSlice16)

	// Field 17: BlockAccessList
	c.BlockAccessList = append([]byte{}, sszSlice17...)

	// Field 18: SlotNumber
	c.SlotNumber = binary.LittleEndian.Uint64(sszSlice18)
	return err
}

func (c *GloasExecutionPayload) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasExecutionPayload) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasExecutionPayload = []byte{0b11111111, 0b11111111, 0b00000111}

func (c *GloasExecutionPayload) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasExecutionPayload) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentHash
	if len(c.ParentHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentHash[:])
	// Field 1: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient[:])
	// Field 2: StateRoot
	if len(c.StateRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.StateRoot[:])
	// Field 3: ReceiptsRoot
	if len(c.ReceiptsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ReceiptsRoot[:])
	// Field 4: LogsBloom
	if len(c.LogsBloom) != 256 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.LogsBloom[:])
	// Field 5: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao[:])
	// Field 6: BlockNumber
	hh.PutUint64(c.BlockNumber)
	// Field 7: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 8: GasUsed
	hh.PutUint64(c.GasUsed)
	// Field 9: Timestamp
	hh.PutUint64(c.Timestamp)
	// Field 10: ExtraData

	{
		if len(c.ExtraData) > 32 {
			return ssz.ErrBytesLength
		}
		subIndx := hh.Index()
		hh.AppendBytes32(c.ExtraData)
		numItems := uint64(len(c.ExtraData))
		hh.MerkleizeWithMixin(subIndx, numItems, (32*1+31)/32)
	}

	// Field 11: BaseFeePerGas
	if len(c.BaseFeePerGas) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BaseFeePerGas[:])
	// Field 12: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash[:])
	// Field 13: Transactions
	{
		subIndx := hh.Index()
		for _, o := range c.Transactions {
			{
				subIndx := hh.Index()
				hh.AppendBytes32(o)
				hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(o)))
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Transactions)))
	}
	// Field 14: Withdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Withdrawals)))
	}
	// Field 15: BlobGasUsed
	hh.PutUint64(c.BlobGasUsed)
	// Field 16: ExcessBlobGas
	hh.PutUint64(c.ExcessBlobGas)
	// Field 17: BlockAccessList
	{
		subIndx := hh.Index()
		hh.AppendBytes32(c.BlockAccessList)
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BlockAccessList)))
	}
	// Field 18: SlotNumber
	hh.PutUint64(c.SlotNumber)
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasExecutionPayload)
	return nil
}

func (c *GloasExecutionPayloadBid) SizeSSZ() int {
	size := 224
	size += len(c.BlobKZGCommitments) * 48
	return size
}

func (c *GloasExecutionPayloadBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasExecutionPayloadBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 224

	// Field 0: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBlockHash[:]...)

	// Field 1: ParentBlockRoot
	if len(c.ParentBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBlockRoot[:]...)

	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BlockHash[:]...)

	// Field 3: PrevRandao
	if len(c.PrevRandao) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.PrevRandao[:]...)

	// Field 4: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.FeeRecipient[:]...)

	// Field 5: GasLimit
	dst = binary.LittleEndian.AppendUint64(dst, c.GasLimit)

	// Field 6: BuilderIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.BuilderIndex)

	// Field 7: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 8: Value
	dst = binary.LittleEndian.AppendUint64(dst, c.Value)

	// Field 9: ExecutionPayment
	dst = binary.LittleEndian.AppendUint64(dst, c.ExecutionPayment)

	// Field 10: BlobKZGCommitments
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BlobKZGCommitments) * 48

	// Field 11: ExecutionRequestsRoot
	if len(c.ExecutionRequestsRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ExecutionRequestsRoot[:]...)

	// Field 10: BlobKZGCommitments
	for _, o := range c.BlobKZGCommitments {
		if len(o) != 48 {
			return nil, ssz.ErrBytesLength
		}
		dst = append(dst, o[:]...)
	}
	return dst, err
}

func (c *GloasExecutionPayloadBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 224 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]     // c.ParentBlockHash
	sszSlice1 := buf[32:64]    // c.ParentBlockRoot
	sszSlice2 := buf[64:96]    // c.BlockHash
	sszSlice3 := buf[96:128]   // c.PrevRandao
	sszSlice4 := buf[128:148]  // c.FeeRecipient
	sszSlice5 := buf[148:156]  // c.GasLimit
	sszSlice6 := buf[156:164]  // c.BuilderIndex
	sszSlice7 := buf[164:172]  // c.Slot
	sszSlice8 := buf[172:180]  // c.Value
	sszSlice9 := buf[180:188]  // c.ExecutionPayment
	sszSlice11 := buf[192:224] // c.ExecutionRequestsRoot

	sszVarOffset10 := ssz.ReadOffset(buf[188:192]) // c.BlobKZGCommitments
	if sszVarOffset10 != 224 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset10 > size {
		return ssz.ErrOffset
	}
	sszSlice10 := buf[sszVarOffset10:] // c.BlobKZGCommitments

	// Field 0: ParentBlockHash
	copy(c.ParentBlockHash[:], sszSlice0)

	// Field 1: ParentBlockRoot
	copy(c.ParentBlockRoot[:], sszSlice1)

	// Field 2: BlockHash
	copy(c.BlockHash[:], sszSlice2)

	// Field 3: PrevRandao
	copy(c.PrevRandao[:], sszSlice3)

	// Field 4: FeeRecipient
	copy(c.FeeRecipient[:], sszSlice4)

	// Field 5: GasLimit
	c.GasLimit = binary.LittleEndian.Uint64(sszSlice5)

	// Field 6: BuilderIndex
	c.BuilderIndex = binary.LittleEndian.Uint64(sszSlice6)

	// Field 7: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice7)

	// Field 8: Value
	c.Value = binary.LittleEndian.Uint64(sszSlice8)

	// Field 9: ExecutionPayment
	c.ExecutionPayment = binary.LittleEndian.Uint64(sszSlice9)

	// Field 10: BlobKZGCommitments
	{
		if len(sszSlice10)%48 != 0 {
			return fmt.Errorf("misaligned bytes: c.BlobKZGCommitments length is %d, which is not a multiple of 48: %w", len(sszSlice10), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice10) / 48
		c.BlobKZGCommitments = make([][48]byte, numElem)
		for i := 0; i < numElem; i++ {
			var tmp [48]byte

			tmpSlice := sszSlice10[i*48 : (1+i)*48]
			copy(tmp[:], tmpSlice)
			c.BlobKZGCommitments[i] = tmp
		}
	}

	// Field 11: ExecutionRequestsRoot
	copy(c.ExecutionRequestsRoot[:], sszSlice11)
	return err
}

func (c *GloasExecutionPayloadBid) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasExecutionPayloadBid) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasExecutionPayloadBid = []byte{0b11111111, 0b00001111}

func (c *GloasExecutionPayloadBid) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasExecutionPayloadBid) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: ParentBlockHash
	if len(c.ParentBlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockHash[:])
	// Field 1: ParentBlockRoot
	if len(c.ParentBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBlockRoot[:])
	// Field 2: BlockHash
	if len(c.BlockHash) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BlockHash[:])
	// Field 3: PrevRandao
	if len(c.PrevRandao) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.PrevRandao[:])
	// Field 4: FeeRecipient
	if len(c.FeeRecipient) != 20 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.FeeRecipient[:])
	// Field 5: GasLimit
	hh.PutUint64(c.GasLimit)
	// Field 6: BuilderIndex
	hh.PutUint64(c.BuilderIndex)
	// Field 7: Slot
	hh.PutUint64(c.Slot)
	// Field 8: Value
	hh.PutUint64(c.Value)
	// Field 9: ExecutionPayment
	hh.PutUint64(c.ExecutionPayment)
	// Field 10: BlobKZGCommitments
	{
		subIndx := hh.Index()
		for _, o := range c.BlobKZGCommitments {
			if len(o) != 48 {
				return ssz.ErrBytesLength
			}
			hh.PutBytes(o[:])
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BlobKZGCommitments)))
	}
	// Field 11: ExecutionRequestsRoot
	if len(c.ExecutionRequestsRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ExecutionRequestsRoot[:])
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasExecutionPayloadBid)
	return nil
}

func (c *GloasExecutionPayloadEnvelope) SizeSSZ() int {
	size := 80
	if c.Payload == nil {
		c.Payload = new(GloasExecutionPayload)
	}
	size += c.Payload.SizeSSZ()
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(GloasExecutionRequests)
	}
	size += c.ExecutionRequests.SizeSSZ()
	return size
}

func (c *GloasExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 80

	// Field 0: Payload
	if c.Payload == nil {
		c.Payload = new(GloasExecutionPayload)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Payload.SizeSSZ()

	// Field 1: ExecutionRequests
	if c.ExecutionRequests == nil {
		c.ExecutionRequests = new(GloasExecutionRequests)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.ExecutionRequests.SizeSSZ()

	// Field 2: BuilderIndex
	dst = binary.LittleEndian.AppendUint64(dst, c.BuilderIndex)

	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot[:]...)

	// Field 4: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.ParentBeaconBlockRoot[:]...)

	// Field 0: Payload
	if dst, err = c.Payload.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Payload: %w", err)
	}

	// Field 1: ExecutionRequests
	if dst, err = c.ExecutionRequests.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("ExecutionRequests: %w", err)
	}
	return dst, err
}

func (c *GloasExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 80 {
		return ssz.ErrSize
	}

	sszSlice2 := buf[8:16]  // c.BuilderIndex
	sszSlice3 := buf[16:48] // c.BeaconBlockRoot
	sszSlice4 := buf[48:80] // c.ParentBeaconBlockRoot

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Payload
	if sszVarOffset0 != 80 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.ExecutionRequests
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Payload
	sszSlice1 := buf[sszVarOffset1:]              // c.ExecutionRequests

	// Field 0: Payload
	c.Payload = new(GloasExecutionPayload)
	if err = c.Payload.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}

	// Field 1: ExecutionRequests
	c.ExecutionRequests = new(GloasExecutionRequests)
	if err = c.ExecutionRequests.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}

	// Field 2: BuilderIndex
	c.BuilderIndex = binary.LittleEndian.Uint64(sszSlice2)

	// Field 3: BeaconBlockRoot
	copy(c.BeaconBlockRoot[:], sszSlice3)

	// Field 4: ParentBeaconBlockRoot
	copy(c.ParentBeaconBlockRoot[:], sszSlice4)
	return err
}

func (c *GloasExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasExecutionPayloadEnvelope = []byte{0b00011111}

func (c *GloasExecutionPayloadEnvelope) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasExecutionPayloadEnvelope) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Payload
	if err := c.Payload.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Payload: %w", err)
	}
	// Field 1: ExecutionRequests
	if err := c.ExecutionRequests.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("ExecutionRequests: %w", err)
	}
	// Field 2: BuilderIndex
	hh.PutUint64(c.BuilderIndex)
	// Field 3: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot[:])
	// Field 4: ParentBeaconBlockRoot
	if len(c.ParentBeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.ParentBeaconBlockRoot[:])
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasExecutionPayloadEnvelope)
	return nil
}

func (c *GloasExecutionRequests) SizeSSZ() int {
	size := 20
	size += len(c.Deposits) * 192
	size += len(c.Withdrawals) * 76
	size += len(c.Consolidations) * 116
	size += len(c.BuilderDeposits) * 184
	size += len(c.BuilderExits) * 68
	return size
}

func (c *GloasExecutionRequests) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasExecutionRequests) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 20

	// Field 0: Deposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Deposits) * 192

	// Field 1: Withdrawals
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Withdrawals) * 76

	// Field 2: Consolidations
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.Consolidations) * 116

	// Field 3: BuilderDeposits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderDeposits) * 184

	// Field 4: BuilderExits
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.BuilderExits) * 68

	// Field 0: Deposits
	for _, o := range c.Deposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Deposits: %w", err)
		}
	}

	// Field 1: Withdrawals
	for _, o := range c.Withdrawals {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Withdrawals: %w", err)
		}
	}

	// Field 2: Consolidations
	for _, o := range c.Consolidations {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("Consolidations: %w", err)
		}
	}

	// Field 3: BuilderDeposits
	for _, o := range c.BuilderDeposits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderDeposits: %w", err)
		}
	}

	// Field 4: BuilderExits
	for _, o := range c.BuilderExits {
		if dst, err = o.MarshalSSZTo(dst); err != nil {
			return nil, fmt.Errorf("BuilderExits: %w", err)
		}
	}
	return dst, err
}

func (c *GloasExecutionRequests) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 20 {
		return ssz.ErrSize
	}

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Deposits
	if sszVarOffset0 != 20 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszVarOffset1 := ssz.ReadOffset(buf[4:8]) // c.Withdrawals
	if sszVarOffset1 > size || sszVarOffset1 < sszVarOffset0 {
		return ssz.ErrOffset
	}
	sszVarOffset2 := ssz.ReadOffset(buf[8:12]) // c.Consolidations
	if sszVarOffset2 > size || sszVarOffset2 < sszVarOffset1 {
		return ssz.ErrOffset
	}
	sszVarOffset3 := ssz.ReadOffset(buf[12:16]) // c.BuilderDeposits
	if sszVarOffset3 > size || sszVarOffset3 < sszVarOffset2 {
		return ssz.ErrOffset
	}
	sszVarOffset4 := ssz.ReadOffset(buf[16:20]) // c.BuilderExits
	if sszVarOffset4 > size || sszVarOffset4 < sszVarOffset3 {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:sszVarOffset1] // c.Deposits
	sszSlice1 := buf[sszVarOffset1:sszVarOffset2] // c.Withdrawals
	sszSlice2 := buf[sszVarOffset2:sszVarOffset3] // c.Consolidations
	sszSlice3 := buf[sszVarOffset3:sszVarOffset4] // c.BuilderDeposits
	sszSlice4 := buf[sszVarOffset4:]              // c.BuilderExits

	// Field 0: Deposits
	{
		if len(sszSlice0)%192 != 0 {
			return fmt.Errorf("misaligned bytes: c.Deposits length is %d, which is not a multiple of 192: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 192
		c.Deposits = make([]*ElectraDepositRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraDepositRequest
			tmp = new(ElectraDepositRequest)
			tmpSlice := sszSlice0[i*192 : (1+i)*192]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
			c.Deposits[i] = tmp
		}
	}

	// Field 1: Withdrawals
	{
		if len(sszSlice1)%76 != 0 {
			return fmt.Errorf("misaligned bytes: c.Withdrawals length is %d, which is not a multiple of 76: %w", len(sszSlice1), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice1) / 76
		c.Withdrawals = make([]*ElectraWithdrawalRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraWithdrawalRequest
			tmp = new(ElectraWithdrawalRequest)
			tmpSlice := sszSlice1[i*76 : (1+i)*76]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
			c.Withdrawals[i] = tmp
		}
	}

	// Field 2: Consolidations
	{
		if len(sszSlice2)%116 != 0 {
			return fmt.Errorf("misaligned bytes: c.Consolidations length is %d, which is not a multiple of 116: %w", len(sszSlice2), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice2) / 116
		c.Consolidations = make([]*ElectraConsolidationRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *ElectraConsolidationRequest
			tmp = new(ElectraConsolidationRequest)
			tmpSlice := sszSlice2[i*116 : (1+i)*116]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
			c.Consolidations[i] = tmp
		}
	}

	// Field 3: BuilderDeposits
	{
		if len(sszSlice3)%184 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderDeposits length is %d, which is not a multiple of 184: %w", len(sszSlice3), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice3) / 184
		c.BuilderDeposits = make([]*GloasBuilderDepositRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *GloasBuilderDepositRequest
			tmp = new(GloasBuilderDepositRequest)
			tmpSlice := sszSlice3[i*184 : (1+i)*184]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderDeposits: %w", err)
			}
			c.BuilderDeposits[i] = tmp
		}
	}

	// Field 4: BuilderExits
	{
		if len(sszSlice4)%68 != 0 {
			return fmt.Errorf("misaligned bytes: c.BuilderExits length is %d, which is not a multiple of 68: %w", len(sszSlice4), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice4) / 68
		c.BuilderExits = make([]*GloasBuilderExitRequest, numElem)
		for i := 0; i < numElem; i++ {
			var tmp *GloasBuilderExitRequest
			tmp = new(GloasBuilderExitRequest)
			tmpSlice := sszSlice4[i*68 : (1+i)*68]
			if err = tmp.UnmarshalSSZ(tmpSlice); err != nil {
				return fmt.Errorf("BuilderExits: %w", err)
			}
			c.BuilderExits[i] = tmp
		}
	}
	return err
}

func (c *GloasExecutionRequests) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasExecutionRequests) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasExecutionRequests = []byte{0b00011111}

func (c *GloasExecutionRequests) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasExecutionRequests) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Deposits
	{
		subIndx := hh.Index()
		for _, o := range c.Deposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Deposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Deposits)))
	}
	// Field 1: Withdrawals
	{
		subIndx := hh.Index()
		for _, o := range c.Withdrawals {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Withdrawals: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Withdrawals)))
	}
	// Field 2: Consolidations
	{
		subIndx := hh.Index()
		for _, o := range c.Consolidations {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("Consolidations: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.Consolidations)))
	}
	// Field 3: BuilderDeposits
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderDeposits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderDeposits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderDeposits)))
	}
	// Field 4: BuilderExits
	{
		subIndx := hh.Index()
		for _, o := range c.BuilderExits {
			if err := o.HashTreeRootWith(hh); err != nil {
				return fmt.Errorf("BuilderExits: %w", err)
			}
		}
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.BuilderExits)))
	}
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasExecutionRequests)
	return nil
}

func (c *GloasIndexedAttestation) SizeSSZ() int {
	size := 228
	size += len(c.AttestingIndices) * 8
	return size
}

func (c *GloasIndexedAttestation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasIndexedAttestation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 228

	// Field 0: AttestingIndices
	dst = ssz.WriteOffset(dst, offset)
	offset += len(c.AttestingIndices) * 8

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(AttestationData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 0: AttestingIndices
	for _, o := range c.AttestingIndices {
		dst = binary.LittleEndian.AppendUint64(dst, o)
	}
	return dst, err
}

func (c *GloasIndexedAttestation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 228 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:132]   // c.Data
	sszSlice2 := buf[132:228] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.AttestingIndices
	if sszVarOffset0 != 228 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.AttestingIndices

	// Field 0: AttestingIndices
	{
		if len(sszSlice0)%8 != 0 {
			return fmt.Errorf("misaligned bytes: c.AttestingIndices length is %d, which is not a multiple of 8: %w", len(sszSlice0), ssz.ErrIncorrectListSize)
		}
		numElem := len(sszSlice0) / 8
		c.AttestingIndices = make([]uint64, numElem)
		for i := 0; i < numElem; i++ {
			var tmp uint64

			tmpSlice := sszSlice0[i*8 : (1+i)*8]
			tmp = binary.LittleEndian.Uint64(tmpSlice)
			c.AttestingIndices[i] = tmp
		}
	}

	// Field 1: Data
	c.Data = new(AttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	copy(c.Signature[:], sszSlice2)
	return err
}

func (c *GloasIndexedAttestation) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasIndexedAttestation) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasIndexedAttestation = []byte{0b00000111}

func (c *GloasIndexedAttestation) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasIndexedAttestation) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AttestingIndices
	{
		subIndx := hh.Index()
		for _, o := range c.AttestingIndices {
			hh.AppendUint64(o)
		}
		hh.FillUpTo32()
		hh.MerkleizeProgressiveWithMixin(subIndx, uint64(len(c.AttestingIndices)))
	}
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasIndexedAttestation)
	return nil
}

func (c *GloasPayloadAttestation) SizeSSZ() int {
	size := 202

	return size
}

func (c *GloasPayloadAttestation) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasPayloadAttestation) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: AggregationBits
	if len([]byte(c.AggregationBits)) != 64 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, []byte(c.AggregationBits)...)

	// Field 1: Data
	if c.Data == nil {
		c.Data = new(GloasPayloadAttestationData)
	}
	if dst, err = c.Data.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	return dst, err
}

func (c *GloasPayloadAttestation) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 202 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:64]    // c.AggregationBits
	sszSlice1 := buf[64:106]  // c.Data
	sszSlice2 := buf[106:202] // c.Signature

	// Field 0: AggregationBits
	c.AggregationBits = make([]byte, 0, 64)
	c.AggregationBits = append(c.AggregationBits, go_bitfield.Bitvector512(sszSlice0)...)

	// Field 1: Data
	c.Data = new(GloasPayloadAttestationData)
	if err = c.Data.UnmarshalSSZ(sszSlice1); err != nil {
		return fmt.Errorf("Data: %w", err)
	}

	// Field 2: Signature
	copy(c.Signature[:], sszSlice2)
	return err
}

func (c *GloasPayloadAttestation) HashTreeRoot() ([32]byte, error) {
	return c.ProgressiveHashTreeRoot()
}

func (c *GloasPayloadAttestation) HashTreeRootWith(hh *ssz.Hasher) error {
	return c.ProgressiveHashTreeRootWith(hh)
}

var activeFieldsGloasPayloadAttestation = []byte{0b00000111}

func (c *GloasPayloadAttestation) ProgressiveHashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.ProgressiveHashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasPayloadAttestation) ProgressiveHashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: AggregationBits
	if len([]byte(c.AggregationBits)) != 64 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes([]byte(c.AggregationBits))
	// Field 1: Data
	if err := c.Data.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Data: %w", err)
	}
	// Field 2: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.MerkleizeProgressiveWithActiveFields(indx, activeFieldsGloasPayloadAttestation)
	return nil
}

func (c *GloasPayloadAttestationData) SizeSSZ() int {
	size := 42

	return size
}

func (c *GloasPayloadAttestationData) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasPayloadAttestationData) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error

	// Field 0: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.BeaconBlockRoot[:]...)

	// Field 1: Slot
	dst = binary.LittleEndian.AppendUint64(dst, c.Slot)

	// Field 2: PayloadPresent
	if c.PayloadPresent {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	// Field 3: BlobDataAvailable
	if c.BlobDataAvailable {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}

	return dst, err
}

func (c *GloasPayloadAttestationData) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size != 42 {
		return ssz.ErrSize
	}

	sszSlice0 := buf[0:32]  // c.BeaconBlockRoot
	sszSlice1 := buf[32:40] // c.Slot
	sszSlice2 := buf[40:41] // c.PayloadPresent
	sszSlice3 := buf[41:42] // c.BlobDataAvailable

	// Field 0: BeaconBlockRoot
	copy(c.BeaconBlockRoot[:], sszSlice0)

	// Field 1: Slot
	c.Slot = binary.LittleEndian.Uint64(sszSlice1)

	// Field 2: PayloadPresent
	if sszSlice2[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice2[0] == 1 {
		c.PayloadPresent = true
	} else {
		c.PayloadPresent = false
	}

	// Field 3: BlobDataAvailable
	if sszSlice3[0] > 1 {
		return ssz.ErrInvalidSerialization
	}
	if sszSlice3[0] == 1 {
		c.BlobDataAvailable = true
	} else {
		c.BlobDataAvailable = false
	}
	return err
}

func (c *GloasPayloadAttestationData) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasPayloadAttestationData) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: BeaconBlockRoot
	if len(c.BeaconBlockRoot) != 32 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.BeaconBlockRoot[:])
	// Field 1: Slot
	hh.PutUint64(c.Slot)
	// Field 2: PayloadPresent
	hh.PutBool(c.PayloadPresent)
	// Field 3: BlobDataAvailable
	hh.PutBool(c.BlobDataAvailable)
	hh.Merkleize(indx)
	return nil
}

func (c *GloasSignedBeaconBlock) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(GloasBeaconBlock)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *GloasSignedBeaconBlock) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasSignedBeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(GloasBeaconBlock)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *GloasSignedBeaconBlock) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:100] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 100 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(GloasBeaconBlock)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *GloasSignedBeaconBlock) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasSignedBeaconBlock) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *GloasSignedExecutionPayloadBid) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(GloasExecutionPayloadBid)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *GloasSignedExecutionPayloadBid) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasSignedExecutionPayloadBid) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(GloasExecutionPayloadBid)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *GloasSignedExecutionPayloadBid) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:100] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 100 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(GloasExecutionPayloadBid)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *GloasSignedExecutionPayloadBid) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasSignedExecutionPayloadBid) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}

func (c *GloasSignedExecutionPayloadEnvelope) SizeSSZ() int {
	size := 100
	if c.Message == nil {
		c.Message = new(GloasExecutionPayloadEnvelope)
	}
	size += c.Message.SizeSSZ()
	return size
}

func (c *GloasSignedExecutionPayloadEnvelope) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, c.SizeSSZ())
	return c.MarshalSSZTo(buf[:0])
}

func (c *GloasSignedExecutionPayloadEnvelope) MarshalSSZTo(dst []byte) ([]byte, error) {
	var err error
	offset := 100

	// Field 0: Message
	if c.Message == nil {
		c.Message = new(GloasExecutionPayloadEnvelope)
	}
	dst = ssz.WriteOffset(dst, offset)
	offset += c.Message.SizeSSZ()

	// Field 1: Signature
	if len(c.Signature) != 96 {
		return nil, ssz.ErrBytesLength
	}
	dst = append(dst, c.Signature[:]...)

	// Field 0: Message
	if dst, err = c.Message.MarshalSSZTo(dst); err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	return dst, err
}

func (c *GloasSignedExecutionPayloadEnvelope) UnmarshalSSZ(buf []byte) error {
	var err error
	size := uint64(len(buf))
	if size < 100 {
		return ssz.ErrSize
	}

	sszSlice1 := buf[4:100] // c.Signature

	sszVarOffset0 := ssz.ReadOffset(buf[0:4]) // c.Message
	if sszVarOffset0 != 100 {
		return ssz.ErrInvalidVariableOffset
	}
	if sszVarOffset0 > size {
		return ssz.ErrOffset
	}
	sszSlice0 := buf[sszVarOffset0:] // c.Message

	// Field 0: Message
	c.Message = new(GloasExecutionPayloadEnvelope)
	if err = c.Message.UnmarshalSSZ(sszSlice0); err != nil {
		return fmt.Errorf("Message: %w", err)
	}

	// Field 1: Signature
	copy(c.Signature[:], sszSlice1)
	return err
}

func (c *GloasSignedExecutionPayloadEnvelope) HashTreeRoot() ([32]byte, error) {
	hh := ssz.DefaultHasherPool.Get()
	if err := c.HashTreeRootWith(hh); err != nil {
		ssz.DefaultHasherPool.Put(hh)
		return [32]byte{}, err
	}
	root, err := hh.HashRoot()
	ssz.DefaultHasherPool.Put(hh)
	return root, err
}

func (c *GloasSignedExecutionPayloadEnvelope) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()
	// Field 0: Message
	if err := c.Message.HashTreeRootWith(hh); err != nil {
		return fmt.Errorf("Message: %w", err)
	}
	// Field 1: Signature
	if len(c.Signature) != 96 {
		return ssz.ErrBytesLength
	}
	hh.PutBytes(c.Signature[:])
	hh.Merkleize(indx)
	return nil
}
