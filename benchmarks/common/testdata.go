package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// TestData holds the loaded test data
type TestData struct {
	BlockMainnet []byte
	BlockMinimal []byte
	StateMainnet []byte
	StateMinimal []byte
}

// LoadTestData loads all test data files from the res directory
func LoadTestData(resDir string) (*TestData, error) {
	td := &TestData{}

	var err error
	td.BlockMainnet, err = os.ReadFile(filepath.Join(resDir, "block-mainnet.ssz"))
	if err != nil {
		return nil, fmt.Errorf("failed to load block-mainnet.ssz: %w", err)
	}

	td.BlockMinimal, err = os.ReadFile(filepath.Join(resDir, "block-minimal.ssz"))
	if err != nil {
		return nil, fmt.Errorf("failed to load block-minimal.ssz: %w", err)
	}

	td.StateMainnet, err = os.ReadFile(filepath.Join(resDir, "state-mainnet.ssz"))
	if err != nil {
		return nil, fmt.Errorf("failed to load state-mainnet.ssz: %w", err)
	}

	td.StateMinimal, err = os.ReadFile(filepath.Join(resDir, "state-minimal.ssz"))
	if err != nil {
		return nil, fmt.Errorf("failed to load state-minimal.ssz: %w", err)
	}

	return td, nil
}
