package service

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"starhub/metric"
	"starhub/model"
	"sync"
	"sync/atomic"
	"time"
)

type productionCacheKey struct {
	Minute int64
	Batch  string
	Digest [32]byte
}
type productionCacheEntry struct {
	Key  productionCacheKey
	Data []byte
}

var productionCache = struct {
	sync.Mutex
	entries map[productionCacheKey]*list.Element
	order   *list.List
	bytes   int
}{entries: map[productionCacheKey]*list.Element{}, order: list.New()}
var productionCacheHits, productionCacheMisses atomic.Int64

func cachedProductionBucket(at int64, row model.ProductionTelemetryMinute, exists bool, now time.Time) (map[string]any, error) {
	key := productionCacheKey{at, row.BatchID, sha256.Sum256([]byte(row.Payload))}
	cacheable := exists && at < now.Unix()/60*60
	var raw []byte
	if cacheable {
		productionCache.Lock()
		if item := productionCache.entries[key]; item != nil {
			productionCache.order.MoveToFront(item)
			raw = item.Value.(productionCacheEntry).Data
		}
		productionCache.Unlock()
	}
	if raw == nil {
		productionCacheMisses.Add(1)
		var b metric.ProductionMinute
		if exists && (json.Unmarshal([]byte(row.Payload), &b) != nil || b.Minute != row.Minute || b.BatchID != row.BatchID || b.Complete != row.Complete) {
			return nil, errors.New("corrupt production minute")
		}
		output, err := buildProductionBucket(at, b, exists, now)
		if err != nil {
			return nil, err
		}
		raw, err = json.Marshal(output)
		if err != nil {
			return nil, err
		}
		if cacheable && len(raw) <= 1<<20 {
			productionCache.Lock()
			if productionCache.entries[key] == nil {
				productionCache.entries[key] = productionCache.order.PushFront(productionCacheEntry{key, raw})
				productionCache.bytes += len(raw)
				for productionCache.bytes > 16<<20 || productionCache.order.Len() > 2048 {
					last := productionCache.order.Back()
					entry := last.Value.(productionCacheEntry)
					delete(productionCache.entries, entry.Key)
					productionCache.bytes -= len(entry.Data)
					productionCache.order.Remove(last)
				}
			}
			productionCache.Unlock()
		}
	} else {
		productionCacheHits.Add(1)
	}
	// Return a detached tree; callers cannot mutate a cached manifest.
	var output struct {
		Minute int64            `json:"minute"`
		Key    string           `json:"key"`
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, err
	}
	return map[string]any{"minute": output.Minute, "key": output.Key, "models": output.Models}, nil
}
