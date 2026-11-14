package model

import (
	"starhub/metric"
	"time"

	"gorm.io/gorm"
)

var localRoutingWriterWait, localRoutingTransaction metric.DurationHistogram

// The database is the serialization point for durable circuit transitions.
// Obtain its writer before reading state, avoiding read-to-write upgrades.
// Snapshot replacement is fenced only after writer acquisition, so waiting on
// another SQLite writer does not hold the snapshot's read lock.
func withLocalRoutingWrite(item *RouteItem, fn func(*gorm.DB) error) error {
	started := time.Now()
	defer func() { localRoutingTransaction.Observe(time.Since(started)) }()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockRoutingWriter(tx); err != nil {
			return err
		}
		localRoutingWriterWait.Observe(time.Since(started))
		routingSnapshotState.RLock()
		defer routingSnapshotState.RUnlock()
		if item != nil && !routingItemCurrentLocked(*item) {
			return nil
		}
		return fn(tx)
	})
}

func LocalRoutingTimings() map[string]metric.DurationSnapshot {
	return map[string]metric.DurationSnapshot{"routing_writer_wait": localRoutingWriterWait.Snapshot(), "routing_transaction": localRoutingTransaction.Snapshot()}
}
