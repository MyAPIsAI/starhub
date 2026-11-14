package metric

import "sync/atomic"

var ProductionWork = make(chan struct{}, 1)
var productionStageCursor atomic.Uint64

func signalProductionWork() {
	select {
	case ProductionWork <- struct{}{}:
	default:
	}
}

type ProductionStage struct {
	Minute   int64
	BatchID  string
	Shard    int
	Attempts []ProductionAttempt
}

// NextProductionStage copies a bounded prefix while holding just one shard.
// The persisted manifest remains absent until the entire minute is committed.
func NextProductionStage(limit int) *ProductionStage {
	if limit < 1 || limit > 128 {
		limit = 128
	}
	production.RLock()
	defer production.RUnlock()
	var buffer *productionBuffer
	for minute, b := range production.buckets {
		if b.count.Load() > 0 && minute >= production.sealed && (buffer == nil || minute < buffer.view.Minute) {
			buffer = b
		}
	}
	if buffer == nil {
		return nil
	}
	start := int(productionStageCursor.Add(1) % 16)
	for i := 0; i < 16; i++ {
		index := (start + i) % 16
		shard := &buffer.shards[index]
		shard.Lock()
		if len(shard.attempts) == 0 {
			shard.Unlock()
			continue
		}
		stage := &ProductionStage{Minute: buffer.view.Minute, BatchID: buffer.view.BatchID, Shard: index, Attempts: append([]ProductionAttempt(nil), shard.attempts[:min(limit, len(shard.attempts))]...)}
		shard.Unlock()
		return stage
	}
	return nil
}

func AckProductionStage(stage *ProductionStage) {
	production.RLock()
	defer production.RUnlock()
	b := production.buckets[stage.Minute]
	if b == nil || b.view.BatchID != stage.BatchID || stage.Minute < production.sealed || stage.Shard < 0 || stage.Shard >= 16 {
		return
	}
	shard := &b.shards[stage.Shard]
	shard.Lock()
	defer shard.Unlock()
	if len(shard.attempts) < len(stage.Attempts) {
		return
	}
	for i, a := range stage.Attempts {
		if a.ID != shard.attempts[i].ID {
			return
		}
	}
	n := len(stage.Attempts)
	clear(shard.attempts[:n])
	shard.attempts = shard.attempts[n:]
	if len(shard.attempts) == 0 {
		shard.attempts = nil
	}
	b.count.Add(-int64(n))
	production.retained.Add(-int64(n))
}
