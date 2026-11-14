package metric

import (
	"github.com/google/uuid"
	"sort"
	"sync"
	"sync/atomic"
)

type productionShard struct {
	sync.Mutex
	index    map[productionEntryKey]int
	entries  []ProductionEntry
	attempts []ProductionAttempt
}

type productionBuffer struct {
	view              ProductionMinute
	shards            [16]productionShard
	entryCount, count atomic.Int64
	loss              atomic.Pointer[string]
	seal              sync.Once
}

func newProductionBuffer(minute int64) *productionBuffer {
	b := &productionBuffer{view: ProductionMinute{Minute: minute, BatchID: uuid.NewString(), Complete: true, Entries: []ProductionEntry{}, Attempts: []ProductionAttempt{}}}
	for i := range b.shards {
		b.shards[i].index = map[productionEntryKey]int{}
	}
	return b
}

func (b *productionBuffer) markLoss(reason string) { b.loss.CompareAndSwap(nil, &reason) }

func productionShardIndex(key productionEntryKey) uint64 {
	h := uint64(key.channel) ^ 1469598103934665603
	for _, s := range []string{key.model, key.generation} {
		for i := range s {
			h = (h ^ uint64(s[i])) * 1099511628211
		}
	}
	return h % 16
}

// Rotation owns every shard before this executes; sorting and copying never
// hold the collector lock, and sync.Once preserves canonical retry payloads.
func (b *productionBuffer) snapshot() ProductionMinute {
	b.seal.Do(func() {
		if loss := b.loss.Load(); loss != nil {
			b.view.Complete = false
			b.view.Error = *loss
		}
		for i := range b.shards {
			b.view.Entries = append(b.view.Entries, b.shards[i].entries...)
			b.view.Attempts = append(b.view.Attempts, b.shards[i].attempts...)
		}
		sort.Slice(b.view.Entries, func(i, j int) bool {
			a, c := b.view.Entries[i], b.view.Entries[j]
			if a.ChannelID != c.ChannelID {
				return a.ChannelID < c.ChannelID
			}
			if a.ModelName != c.ModelName {
				return a.ModelName < c.ModelName
			}
			return a.Generation < c.Generation
		})
		sort.Slice(b.view.Attempts, func(i, j int) bool { return b.view.Attempts[i].ID < b.view.Attempts[j].ID })
	})
	copy := b.view
	copy.Entries = append([]ProductionEntry{}, b.view.Entries...)
	for i := range copy.Entries {
		e := &copy.Entries[i]
		e.RecentTTFT = append([]int64(nil), e.RecentTTFT...)
		if e.Health != nil {
			health := *e.Health
			e.Health = &health
		}
	}
	copy.Attempts = append([]ProductionAttempt{}, b.view.Attempts...)
	for i := range copy.Attempts {
		copy.Attempts[i].RequestedCapabilities = append([]string(nil), copy.Attempts[i].RequestedCapabilities...)
	}
	return copy
}
