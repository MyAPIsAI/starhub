package observability

import "sync"

type SystemSample struct {
	CPU    *float64 `json:"cpu_percent"`
	Memory *float64 `json:"memory_percent"`
	Disk   *float64 `json:"disk_percent"`
}

var sampleMu sync.Mutex
var lastTotal, lastIdle uint64

func SampleSystem() SystemSample {
	sampleMu.Lock()
	defer sampleMu.Unlock()
	total, idle, memory, disk := systemCounters()
	sample := SystemSample{Memory: memory, Disk: disk}
	if total > lastTotal && lastTotal > 0 && idle >= lastIdle && idle-lastIdle <= total-lastTotal {
		value := 100 * (1 - float64(idle-lastIdle)/float64(total-lastTotal))
		sample.CPU = &value
	}
	lastTotal, lastIdle = total, idle
	return sample
}
