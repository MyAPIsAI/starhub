package observability

import (
	"golang.org/x/sys/unix"
	"os"
	"strconv"
	"strings"
)

func systemCounters() (uint64, uint64, *float64, *float64) {
	var total, idle uint64
	if raw, err := os.ReadFile("/proc/stat"); err == nil {
		fields := strings.Fields(strings.SplitN(string(raw), "\n", 2)[0])
		for i, s := range fields[1:] {
			n, _ := strconv.ParseUint(s, 10, 64)
			if i < 8 {
				total += n
			}
			if i == 3 || i == 4 {
				idle += n
			}
		}
	}
	var memory, disk *float64
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		values := map[string]float64{}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) > 1 {
				values[f[0]], _ = strconv.ParseFloat(f[1], 64)
			}
		}
		if values["MemTotal:"] > 0 {
			v := 100 * (1 - values["MemAvailable:"]/values["MemTotal:"])
			memory = &v
		}
	}
	var stat unix.Statfs_t
	if unix.Statfs(".", &stat) == nil && stat.Blocks > 0 {
		v := 100 * (1 - float64(stat.Bfree)/float64(stat.Blocks))
		disk = &v
	}
	return total, idle, memory, disk
}
