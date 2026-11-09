//go:build !windows && !linux

package observability

func systemCounters() (uint64, uint64, *float64, *float64) { return 0, 0, nil, nil }
