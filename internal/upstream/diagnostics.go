package upstream

import (
	"log"
	"sync/atomic"
	"time"
)

var persistenceFailures atomic.Uint64
var lastPersistenceFailure atomic.Int64

func PersistenceFailed() {
	persistenceFailures.Add(1)
	now := time.Now().Unix()
	previous := lastPersistenceFailure.Swap(now)
	if now-previous >= 60 {
		log.Print("upstream protection persistence degraded; inspect resource health diagnostics")
	}
}

func Diagnostics() map[string]any {
	return map[string]any{"persistence_failures": persistenceFailures.Load(), "last_persistence_failure": lastPersistenceFailure.Load(), "scope": "process"}
}
