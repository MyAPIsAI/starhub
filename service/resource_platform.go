package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"starhub/setting"
	"strconv"
	"strings"
	"sync"
	"time"

	"starhub/model"
)

var resourceHTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type resourceHTTPError int

func (e resourceHTTPError) Error() string {
	return fmt.Sprintf("resource platform returned HTTP %d", int(e))
}

func postResource(ctx context.Context, path string, payload any, result any) error {
	base := strings.TrimRight(os.Getenv("RESOURCE_CATALOG_NOTIFY_URL"), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" {
		return errors.New("invalid resource platform URL")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Protection-Node", os.Getenv("NODE_NAME"))
	response, err := resourceHTTPClient.Do(req)
	if err != nil {
		return errors.New("resource platform transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return resourceHTTPError(response.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
	}
	return nil
}

func NotifyResourceCatalogOnce(ctx context.Context) error {
	if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") == "" || os.Getenv("RESOURCE_CATALOG_NOTIFY_ENABLED") == "0" {
		return nil
	}
	catalog, err := model.ReadPlatformCatalog()
	if err != nil {
		return err
	}
	var row model.PlatformCatalogRecord
	if err := model.DB.First(&row, "revision = ?", catalog.Revision).Error; err != nil {
		return err
	}
	if row.Notified {
		return nil
	}
	// Notifications are invalidation hints; the durable revision is authoritative.
	payload := map[string]any{"event": "catalog.changed", "revision": catalog.Revision, "changed_channels": []int64{}, "changed_models": []string{}, "changed_count": len(catalog.Channels)}
	if err := postResource(ctx, "/internal/catalog/events", payload, nil); err != nil {
		return err
	}
	return model.DB.Model(&model.PlatformCatalogRecord{}).Where("revision <= ?", catalog.Revision).Update("notified", true).Error
}

func RunResourcePlatform(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	wasStale := false
	for {
		if err := NotifyResourceCatalogOnce(ctx); err != nil {
			log.Printf("resource catalog notification failed: %v", err)
		}
		if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" {
			if err := model.QueueRoutingWatermark(time.Now()); err != nil {
				log.Printf("routing watermark persistence failed: %v", err)
			}
		}
		if setting.IsCommercialMode() {
			_, age, ready := model.RoutingSnapshotStatus()
			stale := ready && age > 3*time.Minute
			if stale != wasStale {
				kind, message := "resource_snapshot_recovered", "resource snapshot recovered"
				if stale {
					kind, message = "resource_snapshot_stale", "resource snapshot exceeded three minutes; using constrained fallback routing"
				}
				deliverAlert(AlertEvent{Type: kind, Message: message, Severity: "warning", Timestamp: time.Now().Unix()})
				wasStale = stale
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func DeliverResourceEventsOnce(ctx context.Context) error {
	if err := deliverHardFailureBatch(ctx); err != nil {
		return err
	}
	var rows []model.ResourceEventOutbox
	if err := model.DB.Where("delivered_at = 0 AND stream_sequence = 0 AND next_attempt_at <= ?", time.Now().Unix()).Order("created_at, id").Limit(32).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var ack struct {
			EventID  string `json:"event_id"`
			Accepted bool   `json:"accepted"`
		}
		err := postResource(ctx, row.Path, json.RawMessage(row.Payload), &ack)
		if err == nil && (!ack.Accepted || ack.EventID != row.ID) {
			err = errors.New("resource event acknowledgement mismatch")
		}
		updates := map[string]any{"attempts": row.Attempts + 1, "last_error": "", "delivered_at": time.Now().Unix()}
		if err != nil {
			updates["last_error"] = err.Error()
			updates["delivered_at"] = 0
			updates["next_attempt_at"] = time.Now().Add(model.ResourceEventRetryDelay(row.Attempts + 1)).Unix()
		}
		if dbErr := model.DB.Model(&model.ResourceEventOutbox{}).Where("id = ?", row.ID).Updates(updates).Error; dbErr != nil {
			return dbErr
		}
	}
	return model.DB.Where("delivered_at > 0 AND delivered_at < ?", time.Now().Add(-7*24*time.Hour).Unix()).Delete(&model.ResourceEventOutbox{}).Error
}

func RunLocalRoutingProbesOnce(ctx context.Context) error {
	probes, err := model.ClaimLocalRoutingProbes(time.Now())
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	errorsFound := make(chan error, len(probes))
	for _, probe := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _ := strconv.ParseInt(probe.Item.ChannelID, 10, 64)
			ch, err := model.GetChannelByID(id)
			if err != nil {
				return
			}
			if probe.Item.Protection != nil && model.ChannelEndpointGeneration(ch) != probe.Item.Protection.EndpointGeneration {
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			result := ProbeChannel(probeCtx, ch, probe.Item.ModelName)
			cancel()
			if err := model.CompleteLocalRoutingProbe(probe, result.OK, time.Now(), time.Duration(result.ElapsedMs)*time.Millisecond); err != nil {
				errorsFound <- err
			}
		}()
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		return err
	}
	return nil
}

// Probe latency must never delay catalog invalidations or protection delivery.
func RunLocalRoutingProbes(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if setting.IsCommercialMode() {
			if err := RunLocalRoutingProbesOnce(ctx); err != nil {
				log.Printf("local routing probe failed: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func RunResourceEventDelivery(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" {
			if err := DeliverResourceEventsOnce(ctx); err != nil {
				log.Printf("resource event delivery failed: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func deliverHardFailureBatch(ctx context.Context) error {
	var rows []model.ResourceEventOutbox
	if err := model.DB.Where("delivered_at = 0 AND stream_sequence > 0").Order("stream_started_at, stream_sequence").Limit(64).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 || rows[0].NextAttemptAt > time.Now().Unix() {
		return nil
	}
	payload := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		payload = append(payload, json.RawMessage(row.Payload))
	}
	var acks []struct {
		EventID  string `json:"event_id"`
		Accepted bool   `json:"accepted"`
	}
	err := postResource(ctx, "/internal/routing/hard-failure-events", payload, &acks)
	if err == nil {
		if len(acks) != len(rows) {
			err = errors.New("hard failure batch acknowledgement mismatch")
		} else {
			for i, ack := range acks {
				if !ack.Accepted || ack.EventID != rows[i].ID {
					err = errors.New("hard failure batch acknowledgement mismatch")
					break
				}
			}
		}
	}
	// Retrying the entire ordered batch is safe because event IDs are durable.
	for _, row := range rows {
		updates := map[string]any{"attempts": row.Attempts + 1, "last_error": "", "delivered_at": time.Now().Unix()}
		if err != nil {
			updates["last_error"] = err.Error()
			updates["delivered_at"] = 0
			updates["next_attempt_at"] = time.Now().Add(model.ResourceEventRetryDelay(row.Attempts + 1)).Unix()
		}
		if dbErr := model.DB.Model(&model.ResourceEventOutbox{}).Where("id = ?", row.ID).Updates(updates).Error; dbErr != nil {
			return dbErr
		}
	}
	return nil
}
