package model

// ResourceObservationWork retains the claim ID and exact result through retry
// and restart. An executing row is never replayed against the upstream.
type ResourceObservationWork struct {
	ProbeID            string `gorm:"primaryKey"`
	ResourceKey        string
	ChannelID          int64
	ModelName          string
	EndpointGeneration string
	Phase              string
	Task               string `gorm:"type:text"`
	Result             string `gorm:"type:text"`
	CreatedAt          int64
	UpdatedAt          int64
	LastError          string
}

func RoutingObservationItems() []RouteItem {
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	return append([]RouteItem(nil), routingSnapshotState.items...)
}
