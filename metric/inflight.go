package metric

// InflightCounts reports live spans in this process, independent of persisted
// historical attempts or the remote delivery backlog.
func InflightCounts() (requests, attempts int64) {
	return activeRequests.Load(), activeAttempts.Load()
}
