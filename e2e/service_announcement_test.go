package main

import (
	"fmt"
	"strings"
	"testing"

	"starhub/model"
)

func announcementFixture() map[string]any {
	return map[string]any{"operation_id": "notice-fixture-1", "expected_version": 0, "title": map[string]string{"zh": "服务延迟通知", "en": "Service latency notice", "ar": "إشعار تأخر الخدمة"}, "content": map[string]string{"zh": "部分请求延迟升高。", "en": "Some requests are slower than usual.", "ar": "بعض الطلبات أبطأ من المعتاد."}, "vendors": []string{"Fixture vendor"}, "models": []string{"fixture-model"}, "status": "investigating"}
}

func TestADMAC11OperationsPublishesUpdatesAndResolvesAnnouncement(t *testing.T) {
	handler, root := staffFixture(t)
	visitor := &staffHTTP{t: t, handler: handler, ip: "127.0.0.1:19201"}
	public := visitor.call("GET", "/api/status/announcements", nil, 200)
	if public["total"] != float64(0) {
		t.Fatal("sample notices leaked into public feed")
	}
	visitor.call("GET", "/api/admin/announcements", nil, 401)
	root.enroll("root", "Root-temporary-123")
	var operations *staffHTTP
	for _, role := range []int{10, 20, 5} {
		name := fmt.Sprintf("notice-role-%d", role)
		root.call("POST", "/api/admin/staff", map[string]any{"username": name, "temporary_password": "Notice-temporary-123", "role": role, "reason": "Announcement acceptance"}, 200)
		client := &staffHTTP{t: t, handler: handler, ip: fmt.Sprintf("127.0.0.1:%d", 19210+role)}
		client.call("POST", "/api/admin/auth/login", map[string]string{"username": name, "password": "Notice-temporary-123"}, 200)
		client.call("POST", "/api/admin/announcements", announcementFixture(), 403)
		client.enroll(name, "Notice-temporary-123")
		if role == 10 {
			operations = client
		} else {
			client.call("GET", "/api/admin/announcements", nil, 403)
			client.call("POST", "/api/admin/announcements", announcementFixture(), 403)
			client.call("PUT", "/api/admin/announcements/1", announcementFixture(), 403)
		}
	}
	input := announcementFixture()
	created := operations.call("POST", "/api/admin/announcements", input, 200)
	duplicate := operations.call("POST", "/api/admin/announcements", input, 200)
	if created["id"] != duplicate["id"] || created["version"] != float64(1) {
		t.Fatal("publish retry created a second notice")
	}
	public = visitor.call("GET", "/api/status/announcements", nil, 200)
	if public["total"] != float64(1) {
		t.Fatal("published notice is not public")
	}
	row := public["items"].([]any)[0].(map[string]any)
	for _, language := range []string{"zh", "en", "ar"} {
		if row["title"].(map[string]any)[language] != input["title"].(map[string]string)[language] || row["content"].(map[string]any)[language] != input["content"].(map[string]string)[language] {
			t.Fatal("language content changed")
		}
	}
	if row["creation_key"] != nil || row["operation_id"] != nil || row["created_by"] != nil {
		t.Fatal("internal metadata exposed publicly")
	}
	path := fmt.Sprintf("/api/admin/announcements/%.0f", created["id"])
	input["expected_version"], input["status"] = 1, "monitoring"
	updated := operations.call("PUT", path, input, 200)
	if updated["version"] != float64(2) {
		t.Fatal("notice version did not advance")
	}
	operations.call("PUT", path, input, 409)
	public = visitor.call("GET", "/api/status/announcements", nil, 200)
	if public["items"].([]any)[0].(map[string]any)["status"] != "monitoring" {
		t.Fatal("public notice did not update")
	}
	input["expected_version"], input["status"] = 2, "resolved"
	operations.call("PUT", path, input, 200)
	public = visitor.call("GET", "/api/status/announcements", nil, 200)
	if public["total"] != float64(0) {
		t.Fatal("resolved notice remains active")
	}
	history := root.call("GET", "/api/admin/announcements?status=resolved", nil, 200)
	if history["total"] != float64(1) || history["items"].([]any)[0].(map[string]any)["resolved_at"].(float64) <= 0 {
		t.Fatal("resolution erased history")
	}
	input["expected_version"], input["status"] = 3, "investigating"
	operations.call("PUT", path, input, 409)
	var audits []model.AuditLog
	if err := model.DB.Where("action IN ?", []string{"announcement_published", "announcement_updated", "announcement_resolved"}).Order("id").Find(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if len(audits) != 3 {
		t.Fatalf("expected three atomic audit entries, got %d", len(audits))
	}
	for _, entry := range audits {
		if entry.ActorRole != 10 || entry.Username != "notice-role-10" || entry.IP == "" || entry.ObjectID == "" || entry.AfterSummary == "" || strings.Contains(entry.AfterSummary, "notice-fixture-1") {
			t.Fatalf("invalid audit metadata: %+v", entry)
		}
	}
	bad := announcementFixture()
	bad["title"] = map[string]string{"zh": "Only one translation"}
	operations.call("POST", "/api/admin/announcements", bad, 400)
	bad = announcementFixture()
	bad["vendors"], bad["models"] = []string{}, []string{}
	operations.call("POST", "/api/admin/announcements", bad, 400)
	operations.call("GET", "/api/admin/announcements?page=0", nil, 400)
	operations.call("GET", "/api/admin/announcements?status=unknown", nil, 400)
	root.call("GET", "/api/admin/announcements?page=2", nil, 200)
}
