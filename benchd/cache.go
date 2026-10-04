package main

import (
	"fmt"
	"sync"
)

// doneStamp changes whenever a job finishes.
func (s *store) doneStamp() string {
	var n, last int64
	_ = s.db.QueryRow(`SELECT count(*), coalesce(max(finished), 0) FROM jobs WHERE state = ?`, stateDone).Scan(&n, &last)
	return fmt.Sprintf("%d/%d", n, last)
}

// pageCache keeps built pages until a job finishes.
type pageCache struct {
	mu    sync.Mutex
	stamp string
	pages map[string]any
}

func (c *pageCache) get(stamp, key string, build func() any) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stamp != stamp || c.pages == nil {
		c.stamp, c.pages = stamp, map[string]any{}
	}
	if v, ok := c.pages[key]; ok {
		return v
	}
	v := build()
	c.pages[key] = v
	return v
}

// noiseFloor returns the noise floor, computed anew only after a job
// finished.
func (s *store) noiseFloor(local string) noiseFloor {
	key := local + "/" + s.doneStamp()
	s.nfMu.Lock()
	defer s.nfMu.Unlock()
	if s.nfKey != key {
		s.nf, s.nfKey = noiseFloorOf(s, local), key
	}
	return s.nf
}
