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
	"strings"
	"time"

	"starhub/model"
)

var resourceHTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

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
		return fmt.Errorf("resource platform returned HTTP %d", response.StatusCode)
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
	for {
		if err := NotifyResourceCatalogOnce(ctx); err != nil {
			log.Printf("resource catalog notification failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
