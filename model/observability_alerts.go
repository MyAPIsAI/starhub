package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/observability"
	"starhub/setting"
)

type OperationalAlert struct {
	ID         string  `gorm:"primaryKey;size:64" json:"id"`
	Rule       string  `gorm:"index" json:"rule"`
	Scope      string  `json:"scope"`
	Severity   string  `gorm:"index" json:"severity"`
	Active     bool    `gorm:"index" json:"active"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	FirstSeen  int64   `json:"first_seen"`
	LastSeen   int64   `json:"last_seen"`
	ResolvedAt int64   `json:"resolved_at"`
	Generation int64   `json:"generation"`
}

type AlertDelivery struct {
	ID               string `gorm:"primaryKey;size:32" json:"id"`
	AlertID          string `gorm:"index" json:"alert_id"`
	Channel          string `json:"channel"`
	Severity         string `json:"severity"`
	Transition       string `json:"transition"`
	Payload          string `gorm:"type:text" json:"-"`
	CreatedAt        int64  `json:"created_at"`
	DeliveredAt      int64  `gorm:"index" json:"delivered_at"`
	Attempts         int    `json:"attempts"`
	NextAttempt      int64  `gorm:"index" json:"next_attempt"`
	Lease            string `json:"-"`
	LastError        string `json:"last_error"`
	ReconciliationID int64  `json:"-"`
	Generation       int64  `json:"generation"`
}

func AlertCondition(ctx context.Context, rule, scope, severity string, active bool, value, threshold float64, now time.Time) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return alertCondition(tx, rule, scope, severity, active, value, threshold, now)
	})
}

func alertCondition(tx *gorm.DB, rule, scope, severity string, active bool, value, threshold float64, now time.Time) error {
	if severity != "critical" && severity != "serious" && severity != "general" {
		return errors.New("invalid severity")
	}
	hash := sha256.Sum256([]byte(rule + "\x00" + scope))
	id := hex.EncodeToString(hash[:])
	seed := OperationalAlert{ID: id, Rule: rule, Scope: scope, Severity: severity}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return err
	}
	// Acquire the SQLite write lock before reading the transition.
	if err := tx.Model(&OperationalAlert{}).Where("id = ?", id).UpdateColumn("generation", gorm.Expr("generation")).Error; err != nil {
		return err
	}
	var row OperationalAlert
	if err := tx.First(&row, "id = ?", id).Error; err != nil {
		return err
	}
	changed := row.Active != active
	row.Active, row.Value, row.Threshold, row.LastSeen = active, value, threshold, now.Unix()
	if changed {
		row.Generation++
		transition := "resolved"
		if active {
			row.FirstSeen = now.Unix()
			row.ResolvedAt = 0
			transition = "firing"
		} else {
			row.ResolvedAt = now.Unix()
		}
		payload, _ := json.Marshal(map[string]any{"alert_id": id, "rule": rule, "scope": scope, "severity": severity, "transition": transition, "generation": row.Generation, "occurred_at": now.Unix(), "value": value, "threshold": threshold})
		targets := []string{"general"}
		if severity == "critical" {
			targets = []string{"phone", "im"}
		}
		for _, target := range targets {
			if rule == "reconciliation_difference" && active {
				continue
			}
			delivery := AlertDelivery{ID: observability.NewID(), AlertID: id, Channel: target, Severity: severity, Transition: transition, Payload: string(payload), CreatedAt: now.Unix(), Generation: row.Generation}
			if err := tx.Create(&delivery).Error; err != nil {
				return err
			}
		}
	}
	return tx.Save(&row).Error
}

func NotificationEndpoint(c observability.Config, channel string) string {
	switch channel {
	case "phone":
		return c.PhoneURL
	case "im":
		return c.IMURL
	default:
		if c.GeneralURL != "" {
			return c.GeneralURL
		}
		if c.IMURL != "" {
			return c.IMURL
		}
		return setting.AlertWebhookURLValue()
	}
}

func DeliverOperationalAlerts(ctx context.Context, c observability.Config, now time.Time) error {
	channels := []string{}
	for _, channel := range []string{"phone", "im", "general"} {
		if endpoint := NotificationEndpoint(c, channel); endpoint != "" && observability.ValidateEndpoint(endpoint) == nil {
			channels = append(channels, channel)
		}
	}
	if len(channels) == 0 {
		return nil
	}
	var rows []AlertDelivery
	query := DB.WithContext(ctx).Where("delivered_at = 0 AND next_attempt <= ? AND channel IN ?", now.Unix(), channels)
	if !c.InWorkingHours(now) {
		query = query.Where("severity <> ?", "serious")
	}
	if err := query.Order("created_at, generation, id").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var earlier int64
		if err := DB.WithContext(ctx).Model(&AlertDelivery{}).Where("alert_id = ? AND channel = ? AND generation < ? AND delivered_at = 0", row.AlertID, row.Channel, row.Generation).Count(&earlier).Error; err != nil {
			return err
		}
		if earlier > 0 {
			continue
		}
		if row.Severity == "serious" && !c.InWorkingHours(now) {
			continue
		}
		endpoint := NotificationEndpoint(c, row.Channel)
		if endpoint == "" {
			continue
		}
		if observability.ValidateEndpoint(endpoint) != nil {
			continue
		}
		lease := observability.NewID()
		claim := DB.WithContext(ctx).Model(&AlertDelivery{}).Where("id = ? AND delivered_at = 0 AND next_attempt <= ?", row.ID, now.Unix()).Updates(map[string]any{"lease": lease, "next_attempt": now.Add(time.Minute).Unix(), "attempts": gorm.Expr("attempts + 1")})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(row.Payload))
		code := "notification_unreachable"
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", row.ID)
			if token := c.NotificationToken(row.Channel); token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			response, sendErr := observability.HTTPClient().Do(req)
			if sendErr == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
				response.Body.Close()
				code = "notification_rejected"
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					code = ""
				}
			}
		}
		updates := map[string]any{"lease": "", "last_error": code, "next_attempt": now.Add(time.Duration(min(300, 5<<min(row.Attempts, 6))) * time.Second).Unix()}
		if code == "" {
			updates["delivered_at"] = now.Unix()
		}
		if err := DB.WithContext(ctx).Model(&AlertDelivery{}).Where("id = ? AND lease = ?", row.ID, lease).Updates(updates).Error; err != nil {
			return err
		}
		if code == "" && row.ReconciliationID > 0 {
			if err := DB.WithContext(ctx).Model(&ReconciliationAlert{}).Where("id = ?", row.ReconciliationID).Update("delivered_at", now.Unix()).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func EvaluateOperationalAlerts(ctx context.Context, c observability.Config, now time.Time) error {
	all, err := ReadRequestMetrics(ctx, now, "all")
	if err != nil {
		return err
	}
	if len(all) > 0 && all[0].SuccessRate != nil {
		rate := *all[0].SuccessRate
		if err := AlertCondition(ctx, "platform_success", "platform", "critical", rate < .95, rate, .95, now); err != nil {
			return err
		}
	}
	for _, dimension := range []string{"model", "vendor"} {
		rows, err := ReadRequestMetrics(ctx, now, dimension)
		if err != nil {
			return err
		}
		for _, row := range rows {
			scope := row.Model
			if dimension == "vendor" {
				scope = row.Vendor
			}
			if scope == "" || row.Eligible < c.MinimumSamples || row.SuccessRate == nil {
				continue
			}
			rate := *row.SuccessRate
			if err := AlertCondition(ctx, dimension+"_success", scope, "general", rate < c.ModelSuccess, rate, c.ModelSuccess, now); err != nil {
				return err
			}
		}
	}
	syncView, err := ReadResourceSyncStatus(ctx, now)
	if err != nil {
		return err
	}
	if syncView.Configured {
		if err := AlertCondition(ctx, "sync_alignment", "source", "serious", syncView.Status == "alert", float64(syncView.FailedCycles), 2, now); err != nil {
			return err
		}
	}
	catalog, err := ReadResourceCatalog(ctx)
	if err != nil {
		return err
	}
	if catalog.Snapshot != nil {
		state := resourceSnapshotStatus(catalog.Snapshot, now)
		if state.AgeSeconds != nil {
			if err := AlertCondition(ctx, "snapshot_age", "source", "serious", state.Status == "expired", float64(*state.AgeSeconds), 180, now); err != nil {
				return err
			}
		}
	}
	for _, kind := range []string{"precharge", "settlement", "ledger", "payment_callback"} {
		var count, failed int64
		q := DB.WithContext(ctx).Model(&Observation{}).Where("kind = ? AND at > ? AND at <= ?", kind, now.Add(-5*time.Minute).Unix(), now.Unix())
		if err := q.Count(&count).Error; err != nil {
			return err
		}
		if err := q.Where("success = ?", false).Count(&failed).Error; err != nil {
			return err
		}
		if kind == "payment_callback" {
			if count >= c.MinimumSamples {
				rate := float64(failed) / float64(count)
				if err := AlertCondition(ctx, "payment_failure", "payments", "serious", rate > c.PaymentFailure, rate, c.PaymentFailure, now); err != nil {
					return err
				}
			}
		} else if count > 0 && kind != "precharge" {
			if err := AlertCondition(ctx, kind+"_failure", "accounting", "critical", failed > 0, float64(failed), 0, now); err != nil {
				return err
			}
		}
	}
	var pulses []ObservabilityPulse
	if err := DB.WithContext(ctx).Find(&pulses).Error; err != nil {
		return err
	}
	for _, p := range pulses {
		if err := AlertCondition(ctx, "gateway_unavailable", p.ID, "critical", now.Unix()-p.At > 30, float64(now.Unix()-p.At), 30, now); err != nil {
			return err
		}
		if now.Unix()-p.At > 30 {
			continue
		}
		for kind, value := range map[string]*float64{"cpu": p.CPU, "memory": p.Memory, "disk": p.Disk} {
			if value != nil {
				if err := AlertCondition(ctx, "resource_usage", p.ID+":"+kind, "general", *value > 80, *value, 80, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
