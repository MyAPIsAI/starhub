package model

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/common"
)

// Reports are observational snapshots, never restored into the routing state.
type GatewayResourceReport struct {
	GatewayID string `gorm:"primaryKey;size:64" json:"gateway_id"`
	At        int64  `gorm:"index" json:"at"`
	Resources string `gorm:"type:text" json:"-"`
}

type ResourceSecond struct {
	At        int64 `json:"at"`
	Calls     int64 `json:"calls"`
	Successes int64 `json:"successes"`
}

type ResourceLocalState struct {
	ResourceID    string           `json:"resource_id"`
	Identity      string           `json:"identity"`
	State         string           `json:"state"`
	Failures      int              `json:"consecutive_failures"`
	EWMA          float64          `json:"ewma_ms"`
	OpenUntil     int64            `json:"open_until"`
	DegradedUntil int64            `json:"degraded_until"`
	Seconds       []ResourceSecond `json:"seconds"`
}

type resourceRuntimeEntry struct {
	ResourceLocalState
	resource        ResourceInstance
	channelID       int64
	buckets         [300]ResourceSecond
	slowSamples     int
	normalSuccesses int
	belowLine       bool
	probeBusy       bool
	probeFailures   int
	ewmaBreaches    int
}

type ResourceRuntime struct {
	ID      string
	mu      sync.Mutex
	entries map[string]*resourceRuntimeEntry
	pending []ResourceIncident
}

func NewResourceRuntime(id string) *ResourceRuntime {
	return &ResourceRuntime{ID: id, entries: map[string]*resourceRuntimeEntry{}}
}

var LocalResources = NewResourceRuntime(common.GenerateSessionKey())

func resourceRuntimeIdentity(r ResourceInstance) string {
	return fmt.Sprintf("%s|%s|%s|%s", r.ID, r.ChannelID, r.UpstreamModel, r.CanonicalModel)
}

func (runtime *ResourceRuntime) entry(r ResourceInstance, channel int64) *resourceRuntimeEntry {
	entry := runtime.entries[r.ID]
	identity := resourceRuntimeIdentity(r)
	if entry == nil || entry.Identity != identity {
		entry = &resourceRuntimeEntry{ResourceLocalState: ResourceLocalState{ResourceID: r.ID, Identity: identity, State: "normal"}}
		runtime.entries[r.ID] = entry
	}
	entry.resource = r
	if channel > 0 {
		entry.channelID = channel
	}
	return entry
}

func refreshResourceLocalState(entry *resourceRuntimeEntry, now time.Time) {
	if entry.DegradedUntil > 0 && now.Unix() >= entry.DegradedUntil {
		entry.DegradedUntil, entry.EWMA, entry.slowSamples, entry.normalSuccesses = 0, 0, 0, 0
		entry.belowLine = false
		entry.ewmaBreaches = 0
	}
	entry.State = "normal"
	if entry.DegradedUntil > 0 {
		entry.State = "degraded"
	}
	// An open circuit stays open until a platform probe succeeds.
	if entry.OpenUntil > 0 {
		entry.State = "open"
	}
}

// Attempt counts are per concrete resource, including retries, never customer-call totals.
// Ignored outcomes still count as failed attempts but do not advance either health axis.
func (runtime *ResourceRuntime) Record(r ResourceInstance, channel int64, now time.Time, outcome string, ttftMS float64) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	e := runtime.entry(r, channel)
	refreshResourceLocalState(e, now)
	i := now.Unix() % 300
	if e.buckets[i].At != now.Unix() {
		e.buckets[i] = ResourceSecond{At: now.Unix()}
	}
	e.buckets[i].Calls++
	if outcome == "success" {
		e.buckets[i].Successes++
	}
	if outcome == "failure" {
		e.Failures++
		threshold := 5
		if r.QualityLevel == "U" {
			threshold = 2
		}
		if e.Failures >= threshold && e.OpenUntil == 0 {
			e.OpenUntil = now.Add(30 * time.Second).Unix()
		}
	} else if outcome == "success" {
		e.Failures = 0
		line := map[string]float64{"S0": 1040, "S1": 1950, "S2": 3900}[r.QualityLevel]
		if line > 0 && ttftMS > 0 && (slices.Contains(r.Capabilities, "text") || slices.Contains(r.Capabilities, "code")) {
			e.EWMA = 0.5*ttftMS + 0.5*e.EWMA
			e.slowSamples++
			if e.slowSamples >= 3 && e.EWMA > line {
				e.ewmaBreaches++
				if e.ewmaBreaches == 2 {
					runtime.pending = append(runtime.pending, newResourceIncident("ewma_breach_twice", r.ID, runtime.ID, now))
				}
				if e.DegradedUntil == 0 {
					e.DegradedUntil = now.Add(5 * time.Minute).Unix()
				}
				e.normalSuccesses, e.belowLine = 0, false
			} else if e.DegradedUntil > 0 && e.EWMA <= line {
				e.ewmaBreaches = 0
				if e.belowLine {
					e.normalSuccesses++
				} else {
					e.belowLine = true
				}
				if e.normalSuccesses >= 3 {
					e.DegradedUntil = 0
				}
			}
		}
	}
	refreshResourceLocalState(e, now)
}

func (runtime *ResourceRuntime) State(r ResourceInstance, now time.Time) string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	e := runtime.entry(r, 0)
	refreshResourceLocalState(e, now)
	return e.State
}

type ResourceProbe struct {
	Resource  ResourceInstance
	ChannelID int64
}

func (runtime *ResourceRuntime) DueProbes(now time.Time) []ResourceProbe {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result := []ResourceProbe{}
	for _, e := range runtime.entries {
		if len(result) == 4 {
			break
		}
		if e.OpenUntil > 0 && e.OpenUntil <= now.Unix() && !e.probeBusy && e.channelID > 0 {
			e.probeBusy = true
			result = append(result, ResourceProbe{Resource: e.resource, ChannelID: e.channelID})
		}
	}
	return result
}

func (runtime *ResourceRuntime) FinishProbe(r ResourceInstance, now time.Time, success bool) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	e := runtime.entries[r.ID]
	if e == nil || e.Identity != resourceRuntimeIdentity(r) {
		return
	}
	e.probeBusy = false
	if success {
		e.OpenUntil, e.DegradedUntil, e.Failures, e.probeFailures, e.EWMA, e.slowSamples = 0, 0, 0, 0, 0, 0
		e.ewmaBreaches = 0
	} else {
		e.probeFailures++
		if e.probeFailures == 5 {
			runtime.pending = append(runtime.pending, newResourceIncident("half_open_failed", r.ID, runtime.ID, now))
		}
		seconds := int64(30) << min(e.probeFailures, 4)
		e.OpenUntil = now.Unix() + min(seconds, 300)
	}
	refreshResourceLocalState(e, now)
}

func (runtime *ResourceRuntime) Publish(ctx context.Context, now time.Time) error {
	catalog, err := ReadResourceCatalog(ctx)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	rows := []ResourceLocalState{}
	current := map[string]bool{}
	if catalog.Snapshot != nil {
		for _, r := range catalog.Snapshot.Instances {
			current[r.ID] = true
			e := runtime.entry(r, 0)
			refreshResourceLocalState(e, now)
			row := e.ResourceLocalState
			row.Seconds = []ResourceSecond{}
			for _, b := range e.buckets {
				if b.At > now.Unix()-300 && b.At <= now.Unix() {
					row.Seconds = append(row.Seconds, b)
				}
			}
			rows = append(rows, row)
		}
	}
	for id := range runtime.entries {
		if !current[id] {
			delete(runtime.entries, id)
		}
	}
	pending := slices.Clone(runtime.pending)
	runtime.mu.Unlock()
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	report := GatewayResourceReport{GatewayID: runtime.ID, At: now.Unix(), Resources: string(raw)}
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&report).Error; err != nil {
			return err
		}
		if len(pending) > 0 {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&pending).Error
		}
		return nil
	}); err != nil {
		return err
	}
	if len(pending) > 0 {
		persisted := map[string]bool{}
		for _, event := range pending {
			persisted[event.ID] = true
		}
		runtime.mu.Lock()
		runtime.pending = slices.DeleteFunc(runtime.pending, func(event ResourceIncident) bool { return persisted[event.ID] })
		runtime.mu.Unlock()
	}
	return DB.WithContext(ctx).Where("at < ?", now.Add(-24*time.Hour).Unix()).Delete(&GatewayResourceReport{}).Error
}

func AdmissionResource(ctx context.Context, channel int64, name, group string) (ResourceInstance, bool) {
	evidence, _ := ctx.Value(common.CtxAdmissionRouting).(*RoutingEvidence)
	if evidence == nil {
		return ResourceInstance{}, false
	}
	ids := evidence.Resources[fmt.Sprintf("%d|%s|%s", channel, name, group)]
	if len(ids) != 1 {
		return ResourceInstance{}, false
	}
	r, ok := evidence.Instances[ids[0]]
	return r, ok
}

type ResourceGatewayStatus struct {
	GatewayID     string `json:"gateway_id"`
	LastSeen      int64  `json:"last_seen"`
	State         string `json:"state"`
	Failures      int    `json:"consecutive_failures"`
	OpenUntil     int64  `json:"open_until"`
	DegradedUntil int64  `json:"degraded_until"`
}
type AdminResourceStatus struct {
	ResourceInstance
	Vendor      string                  `json:"vendor"`
	Tiers       []int                   `json:"tiers"`
	Gateways    []ResourceGatewayStatus `json:"gateways"`
	Calls       int64                   `json:"calls_5m"`
	Successes   int64                   `json:"successes_5m"`
	SuccessRate *float64                `json:"success_rate"`
}

func ReadAdminResourceStatus(ctx context.Context, now time.Time) ([]AdminResourceStatus, error) {
	catalog, err := ReadResourceCatalog(ctx)
	if err != nil {
		return nil, err
	}
	reports := []GatewayResourceReport{}
	if err := DB.WithContext(ctx).Where("at >= ?", now.Add(-24*time.Hour).Unix()).Order("gateway_id").Find(&reports).Error; err != nil {
		return nil, err
	}
	states := map[string]map[string]ResourceLocalState{}
	for _, report := range reports {
		var entries []ResourceLocalState
		if err := json.Unmarshal([]byte(report.Resources), &entries); err != nil {
			return nil, err
		}
		states[report.GatewayID] = map[string]ResourceLocalState{}
		for _, entry := range entries {
			states[report.GatewayID][entry.ResourceID] = entry
		}
	}
	rows := []AdminResourceStatus{}
	if catalog.Snapshot == nil {
		return rows, nil
	}
	vendors := map[string]string{}
	for _, channel := range catalog.Snapshot.Channels {
		vendors[channel.ID] = channel.Provider
	}
	for _, r := range catalog.Snapshot.Instances {
		row := AdminResourceStatus{ResourceInstance: r, Vendor: vendors[r.ChannelID], Tiers: []int{}, Gateways: []ResourceGatewayStatus{}}
		text := slices.Contains(r.Capabilities, "text") || slices.Contains(r.Capabilities, "code")
		quality := map[string]int{"S0": 0, "S1": 1, "S2": 2, "U": 3}
		q, known := quality[r.QualityLevel]
		for _, g := range []int{4, 6, 8, 10} {
			limit := map[int]int{4: 2, 6: 2, 8: 1, 10: 0}[g]
			if r.PriceTier != nil && *r.PriceTier <= g && (!text || (known && q <= limit)) {
				row.Tiers = append(row.Tiers, g)
			}
		}
		for _, report := range reports {
			state, exists := states[report.GatewayID][r.ID]
			gateway := ResourceGatewayStatus{GatewayID: report.GatewayID, LastSeen: report.At, State: "unknown"}
			if exists && state.Identity == resourceRuntimeIdentity(r) {
				if now.Unix()-report.At <= 15 {
					gateway.State, gateway.Failures, gateway.OpenUntil, gateway.DegradedUntil = state.State, state.Failures, state.OpenUntil, state.DegradedUntil
				}
				for _, bucket := range state.Seconds {
					if bucket.At > now.Unix()-300 && bucket.At <= now.Unix() {
						row.Calls += bucket.Calls
						row.Successes += bucket.Successes
					}
				}
			}
			row.Gateways = append(row.Gateways, gateway)
		}
		if row.Calls > 0 {
			rate := float64(row.Successes) / float64(row.Calls)
			row.SuccessRate = &rate
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b AdminResourceStatus) int { return compareResourceID(a.ID, b.ID) })
	return rows, nil
}
