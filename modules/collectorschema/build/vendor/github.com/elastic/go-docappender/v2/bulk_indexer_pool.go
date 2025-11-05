// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package docappender

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// BulkIndexerPool is a pool of BulkIndexer instances. It is designed to be
// used in a concurrent environment where multiple goroutines may need to
// acquire and release indexers.
//
// The pool allows a minimum number of BulkIndexers to be guaranteed per ID, a
// maximum number of indexers per ID and an overall lease limit. This is useful
// to ensure the pool does not grow too large, even if some IDs are slow to
// release indexers.
type BulkIndexerPool struct {
	indexers chan *BulkIndexer
	entries  map[string]idEntry
	mu       sync.RWMutex
	cond     *sync.Cond   // To wait/signal when a slot is available.
	leased   atomic.Int64 // Total number of leased indexers across all IDs.

	// Read only fields.
	min, max, total int64
	config          BulkIndexerConfig
}

type idEntry struct {
	nonEmpty chan *BulkIndexer
	leased   *atomic.Int64
}

// NewBulkIndexerPool returns a new BulkIndexerPool with:
// - The specified guaranteed indexers per ID
// - The maximum number of concurrent BulkIndexers per ID
// - A total (max) number of indexers to be leased per pool.
// - The BulkIndexerConfig to use when creating new indexers.
func NewBulkIndexerPool(guaranteed, max, total int, c BulkIndexerConfig) *BulkIndexerPool {
	p := BulkIndexerPool{
		indexers: make(chan *BulkIndexer, total),
		entries:  make(map[string]idEntry),
		min:      int64(guaranteed),
		max:      int64(max),
		total:    int64(total),
		config:   c,
	}
	p.cond = sync.NewCond(&p.mu)
	return &p
}

// Get returns a BulkIndexer for the specified ID as the ID is registered and
// below the guaranteed minimum OR the local and overall limits.
//
// If the overall limit of indexers has been reached, it will wait until a slot
// is available, blocking execution.
//
// If Deregister is called while waiting, an error and nil indexer is returned.
func (p *BulkIndexerPool) Get(ctx context.Context, id string) (*BulkIndexer, error) {
	// Acquire the lock to ensure that we are not racing with other goroutines
	// that may be trying to acquire or release indexers.
	// Even though this looks like it would prevent other Get() operations from
	// proceeding, the lock is only held for a short time while we check the
	// count and overall limits. The lock is released by:
	// - p.cond.Wait() releases the lock while waiting for a signal.
	// - p.mu.Unlock() releases the lock after the indexer is returned.
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		// Get the entry inside the loop in case it is deregistered while
		// waiting, or if the ID is not registered.
		entry, exists := p.entries[id]
		if !exists {
			return nil, fmt.Errorf("bulk indexer pool: id %q not registered", id)
		}
		// Always allow minimum indexers to be leased, regardless of the
		// overall limit. This ensures that the minimum number of indexers
		// are always available for each ID.
		underGuaranteed := entry.leased.Load() < p.min
		if underGuaranteed {
			return p.get(ctx, entry)
		}
		// Only allow indexers to be leased if both the local and overall
		// limits have not been reached.
		underLocalMax := entry.leased.Load() < p.max
		underTotal := p.leased.Load() < p.total
		if underTotal && underLocalMax {
			return p.get(ctx, entry)
		}
		// Waits until Put/Deregister is called. This allows waiting for a
		// slot to become available without busy waiting.
		// When Wait() is called, the mutex is unlocked while waiting.
		// After Wait() returns, the mutex is automatically locked.
		p.cond.Wait()
	}
}

// get returns a BulkIndexer for the specified ID. It is assumed that the
// caller has already acquired the lock (read or write) and checked the count
// and overall limits.
func (p *BulkIndexerPool) get(ctx context.Context, entry idEntry) (
	idx *BulkIndexer, err error,
) {
	defer func() {
		// Only increment the leased count an indexer is returned.
		if idx != nil {
			entry.leased.Add(1)
			p.leased.Add(1)
		}
	}()
	// First, try to return an existing non-empty indexer (if any).
	select {
	case idx = <-entry.nonEmpty:
		return
	default:
	}
	// If there aren't any non-empty indexers, either return an
	// existing indexer or create a new one if none are available.
	select {
	case idx = <-p.indexers:
	case <-ctx.Done():
		err = ctx.Err()
	default:
		idx = newBulkIndexer(p.config)
	}
	return
}

// Put returns the BulkIndexer to the pool. If the indexer is non-empty, it is
// stored in the non-empty channel for the ID. Otherwise, it is returned to the
// general pool.
// After calling Put() no references to the indexer should be stored, since
// doing so may lead to undefined behavior and unintended memory sharing.
func (p *BulkIndexerPool) Put(id string, indexer *BulkIndexer) {
	if indexer == nil {
		return // No indexer to store, nothing to do.
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, exists := p.entries[id]
	if !exists {
		return // unknown id, discard indexer
	}
	defer func() { // Always decrement the count and signal the condition.
		entry.leased.Add(-1)
		p.leased.Add(-1)
		p.cond.Broadcast() // Signal waiting goroutines
	}()
	if indexer.Items() > 0 {
		entry.nonEmpty <- indexer // Never discard non-empty indexers.
		return
	}
	select {
	case p.indexers <- indexer: // Return to the pool for later reuse.
	default:
		indexer = nil // If the pool is full, discard the indexer.
	}
}

// Register adds an ID to the pool. If the ID already exists, it does nothing.
// This is useful for ensuring that the ID is registered before any indexers
// are acquired for it.
func (p *BulkIndexerPool) Register(id string) {
	if id == "" {
		return // No ID to register.
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.entries[id]; exists {
		return
	}
	p.entries[id] = idEntry{
		nonEmpty: make(chan *BulkIndexer, p.max),
		leased:   new(atomic.Int64),
	}
}

// Deregister removes the id from the pool and returns a closed BulkIndexer
// channel with all the non-empty indexers associated with the ID.
func (p *BulkIndexerPool) Deregister(id string) <-chan *BulkIndexer {
	if id == "" {
		return nil // No ID to deregister.
	}
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		p.cond.Broadcast() // Signal ALL waiting goroutines
	}()
	entry, ok := p.entries[id]
	if !ok { // To handle when the ID is not found.
		entry.nonEmpty = make(chan *BulkIndexer)
	}
	delete(p.entries, id)
	close(entry.nonEmpty)
	return entry.nonEmpty
}

func (p *BulkIndexerPool) count(id string) int64 {
	if id == "" {
		return -1
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if entry, exists := p.entries[id]; exists {
		return entry.leased.Load()
	}
	return -1
}
