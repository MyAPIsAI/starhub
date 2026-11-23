package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrBlocked = errors.New("upstream resource is temporarily protected")

type Identity struct {
	ChannelID  int64  `gorm:"index" json:"channel_id"`
	Model      string `gorm:"size:256" json:"model"`
	Capability string `gorm:"size:32" json:"capability"`
	Stream     bool   `json:"stream"`
	Generation string `gorm:"size:64" json:"-"`
	Credential string `gorm:"size:64" json:"-"`
}

func CredentialID(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
func (i Identity) key(scope string) string {
	model, credential := i.Model, i.Credential
	if scope == "credential" {
		model = ""
	}
	if scope == "model" {
		credential = ""
	}
	b, _ := json.Marshal([]any{scope, i.ChannelID, model, credential, i.Generation})
	if scope == "interface" {
		b, _ = json.Marshal([]any{scope, i.ChannelID, model, i.Generation, i.Capability, i.Stream})
	}
	return CredentialID(string(b))
}

type Profile struct {
	Capability string
	Stream     bool
}

func (i Identity) StateID(scope string) string { return i.key(scope) }
func (i Identity) keys() []string {
	return []string{i.key("interface"), i.key("model"), i.key("credential"), i.key("credential_model")}
}

type State struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	Identity    `gorm:"embedded"`
	Scope       string `gorm:"size:24" json:"scope"`
	Failures    int    `json:"failures"`
	Version     uint64 `json:"version"`
	OpenUntil   int64  `gorm:"index" json:"open_until"`
	Backoff     int    `json:"backoff_seconds"`
	Reason      string `gorm:"size:64" json:"reason"`
	ProbeID     string `gorm:"size:36" json:"-"`
	LeaseUntil  int64  `json:"lease_until"`
	ProbeResult string `gorm:"size:32" json:"probe_result"`
	Updated     int64  `gorm:"index" json:"updated_at"`
}

func (State) TableName() string { return "upstream_states" }

type Event struct {
	ID         string `gorm:"primaryKey;size:36" json:"id"`
	RequestID  string `gorm:"index;size:128" json:"request_id"`
	ChannelID  int64  `gorm:"index" json:"channel_id"`
	Model      string `gorm:"size:256" json:"model"`
	ResourceID string `gorm:"index;size:64" json:"resource_id"`
	Kind       string `gorm:"size:16" json:"kind"`
	Class      string `gorm:"size:64" json:"class"`
	Status     int    `json:"status"`
	Sent       bool   `json:"sent"`
	Retry      bool   `json:"retry"`
	Decision   string `gorm:"size:64" json:"decision"`
	OpenUntil  int64  `json:"open_until"`
	Created    int64  `gorm:"index" json:"created_at"`
	Started    int64  `json:"started_at_ms"`
}

func (Event) TableName() string { return "upstream_events" }

type Guard struct {
	ID int `gorm:"primaryKey;autoIncrement:false"`
}

func (Guard) TableName() string { return "upstream_guard" }

// Lock the durable writer before reading, including on SQLite.
func write(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Guard{}).Where("id = 1").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		return fn(tx)
	})
}

type Attempt struct {
	ID      string
	Started int64
	Identity
	Versions map[string]uint64
	Failures map[string]int
}

// Capture rechecks admission immediately before the upstream send.
func Capture(db *gorm.DB, identity Identity, p Policy) (*Attempt, error) {
	a := &Attempt{ID: uuid.NewString(), Started: time.Now().UnixMilli(), Identity: identity, Versions: map[string]uint64{}, Failures: map[string]int{}}
	if !p.Enabled {
		return a, nil
	}
	keys := identity.keys()
	var states []State
	if err := db.Where("id IN ?", keys).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state.OpenUntil != 0 {
			return nil, ErrBlocked
		}
		a.Versions[state.ID] = state.Version
		a.Failures[state.ID] = state.Failures
	}
	return a, nil
}

// Admission checks all credentials in one read and returns only usable keys.
func Admission(db *gorm.DB, identities []Identity, p Policy) (map[string]bool, error) {
	allowed := map[string]bool{}
	blocked, err := Blocked(db, identities, p)
	if err != nil {
		return nil, err
	}
	for _, identity := range identities {
		if !IsBlocked(identity, blocked) {
			allowed[identity.Credential] = true
		}
	}
	return allowed, nil
}

func IsBlocked(identity Identity, blocked map[string]bool) bool {
	for _, id := range identity.keys() {
		if blocked[id] {
			return true
		}
	}
	return false
}

// Blocked batches identities across channels and bounds SQL parameter counts.
func Blocked(db *gorm.DB, identities []Identity, p Policy) (map[string]bool, error) {
	blocked := map[string]bool{}
	if !p.Enabled {
		return blocked, nil
	}
	keys := []string{}
	seen := map[string]bool{}
	for _, identity := range identities {
		for _, id := range identity.keys() {
			if !seen[id] {
				keys = append(keys, id)
				seen[id] = true
			}
		}
	}
	for start := 0; start < len(keys); start += 400 {
		var states []State
		if err := db.Select("id").Where("id IN ? AND open_until <> 0", keys[start:min(start+400, len(keys))]).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, state := range states {
			blocked[state.ID] = true
		}
	}
	return blocked, nil
}

func readState(tx *gorm.DB, identity Identity, scope string) (State, error) {
	s := State{ID: identity.key(scope), Identity: identity, Scope: scope}
	err := tx.First(&s, "id = ?", s.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return s, err
}

// Record atomically persists one attempt and its health transition.
func Record(db *gorm.DB, a *Attempt, v Verdict, event Event, p Policy) (err error) {
	defer func() {
		if err != nil {
			PersistenceFailed()
		}
	}()
	if a == nil {
		return nil
	}
	event.ID, event.ChannelID, event.Model = a.ID, a.ChannelID, a.Model
	event.Started = a.Started
	event.Kind, event.Class, event.Created = "attempt", v.Class, time.Now().Unix()
	if v.Class == "success" {
		event.ResourceID = a.Identity.key("interface")
	}
	// Healthy calls never create or rewrite state rows. CAS preserves failures
	// recorded after this attempt started, without taking the global writer.
	if v.Class == "success" || v.Scope == "" || !p.Enabled || !event.Sent {
		return db.Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
			if result.Error != nil || result.RowsAffected == 0 {
				return result.Error
			}
			if p.Enabled && event.Sent && v.Class == "success" {
				for _, scope := range []string{"model", "interface"} {
					id := a.Identity.key(scope)
					if a.Failures[id] == 0 {
						continue
					}
					if err := tx.Model(&State{}).Where("id = ? AND version = ? AND open_until = 0", id, a.Versions[id]).Updates(map[string]any{"failures": 0, "reason": "recovered", "updated": event.Created}).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	return write(db, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Event{}).Where("id = ?", a.ID).Count(&count).Error; err != nil || count > 0 {
			return err
		}
		s, err := readState(tx, a.Identity, v.Scope)
		if err != nil {
			return err
		}
		event.ResourceID = s.ID
		s.Identity = a.Identity
		s.Version++
		s.Failures = min(s.Failures+1, 1000000)
		s.Reason = v.Class
		s.ProbeID = ""
		// Explicit provider deadlines may extend protection; ordinary late
		// failures must not continually postpone recovery.
		if v.Immediate && s.OpenUntil != 0 {
			s.Backoff = max(s.Backoff, v.Delay)
			s.OpenUntil = max(s.OpenUntil, event.Created+int64(v.Delay))
		}
		if s.OpenUntil == 0 && (v.Immediate || s.Failures >= p.Failures) {
			s.Backoff = p.OpenSeconds
			if v.Immediate {
				s.Backoff = v.Delay
			}
			s.OpenUntil = event.Created + int64(s.Backoff)
		}
		s.Updated = event.Created
		event.OpenUntil = s.OpenUntil
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		return tx.Create(&event).Error
	})
}

// Claim returns a single durable lease; elapsed cooldown never admits production.
func Claim(db *gorm.DB, id string, p Policy) (*State, error) {
	var lease *State
	err := write(db, func(tx *gorm.DB) error {
		var s State
		if err := tx.First(&s, "id = ?", id).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if s.OpenUntil == 0 || s.OpenUntil > now || s.LeaseUntil > now {
			return nil
		}
		id, err := claimSlot(tx, s.Identity, s.ID, p)
		if err != nil {
			return err
		}
		if id == "" {
			// Move deferred candidates behind other due resources so a shared
			// credential cooldown cannot monopolize the scheduler window.
			return tx.Model(&State{}).Where("id = ?", s.ID).Updates(map[string]any{"updated": now, "probe_result": "probe_blocked"}).Error
		}
		s.ProbeID, s.LeaseUntil = id, now+int64(p.ProbeTimeout)+10
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		lease = &s
		return nil
	})
	return lease, err
}

func CompleteProbe(db *gorm.DB, lease State, outcome string, verdict Verdict, status int, p Policy) error {
	return write(db, func(tx *gorm.DB) error {
		if err := tx.Delete(&ProbeSlot{}, "id = ?", lease.ProbeID).Error; err != nil {
			return err
		}
		var s State
		if err := tx.First(&s, "id = ?", lease.ID).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		if s.ProbeID != lease.ProbeID || s.Version != lease.Version || s.LeaseUntil <= now {
			return nil
		}
		s.ProbeID, s.LeaseUntil, s.ProbeResult, s.Updated = "", 0, outcome, now
		s.Version++
		switch outcome {
		case "success":
			s.OpenUntil, s.Backoff, s.Failures = 0, 0, 0
			s.Reason = "recovered"
		case "failure":
			s.Backoff = min(p.MaxOpenSeconds, max(p.OpenSeconds, s.Backoff*2))
			if s.Scope == "credential" || s.Scope == "credential_model" {
				s.Backoff = max(s.Backoff, p.IsolationSeconds)
				if s.Reason == "rate_limited" {
					s.Backoff = max(p.RateSeconds, verdict.Delay)
				}
			}
			if verdict.Immediate {
				s.Backoff = max(s.Backoff, verdict.Delay)
			}
			s.OpenUntil = now + int64(s.Backoff)
			// A probe can discover a wider credential failure. Preserve that
			// protection independently of the state whose recovery was tested.
			if verdict.Immediate && verdict.Scope != "" && verdict.Scope != s.Scope {
				other, err := readState(tx, s.Identity, verdict.Scope)
				if err != nil {
					return err
				}
				other.Version++
				other.Identity, other.Reason = s.Identity, verdict.Class
				other.Failures++
				other.ProbeID = ""
				other.OpenUntil = max(other.OpenUntil, now+int64(verdict.Delay))
				other.Backoff, other.Updated = max(other.Backoff, verdict.Delay), now
				if err := tx.Save(&other).Error; err != nil {
					return err
				}
			}
			if verdict.Scope == s.Scope {
				s.Reason = verdict.Class
			}
		default:
			s.OpenUntil = now + int64(max(p.OpenSeconds, 30))
			if verdict.Class == "probe_not_applicable" {
				s.OpenUntil = now + int64(p.IsolationSeconds)
			}
		}
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		return tx.Create(&Event{ID: lease.ProbeID, ChannelID: s.ChannelID, Model: s.Model, ResourceID: s.ID, Kind: "probe", Class: outcome, Status: status, Decision: verdict.Class, OpenUntil: s.OpenUntil, Created: now}).Error
	})
}

func InitGuard(db *gorm.DB) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Guard{ID: 1}).Error
}

// Reset clears temporary protection only; channel and catalog authority remain unchanged.
func Reset(db *gorm.DB, id string, version uint64, actor int64) error {
	return write(db, func(tx *gorm.DB) error {
		var s State
		if err := tx.First(&s, "id = ?", id).Error; err != nil {
			return err
		}
		if s.Version != version {
			return errors.New("resource protection changed")
		}
		s.Version++
		s.OpenUntil, s.Failures, s.Backoff = 0, 0, 0
		s.ProbeID, s.Reason, s.ProbeResult = "", "manual_reset", "manual_reset"
		s.Updated = time.Now().Unix()
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		return tx.Create(&Event{ID: uuid.NewString(), ChannelID: s.ChannelID, Model: s.Model, ResourceID: s.ID, Kind: "reset", Class: "manual_reset", Decision: fmt.Sprintf("actor:%d", actor), Created: s.Updated, Started: time.Now().UnixMilli()}).Error
	})
}

func Cleanup(ctx context.Context, db *gorm.DB) error {
	db = db.WithContext(ctx)
	if err := db.Where("created < ?", time.Now().Add(-7*24*time.Hour).Unix()).Delete(&Event{}).Error; err != nil {
		return err
	}
	return db.Where("lease_until <= ?", time.Now().Unix()).Delete(&ProbeSlot{}).Error
}
