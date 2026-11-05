package model

import (
	"testing"
	"time"
)

func TestCreateAndGetIncident(t *testing.T) {
	newTestDB(t)
	incident := &ServiceIncident{
		Title:     "Chat API degraded",
		Severity:  IncidentSeverityWarning,
		Status:    IncidentStatusInvestigating,
		Component: "chat",
	}
	if err := CreateIncident(incident); err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}
	if incident.ID == 0 {
		t.Fatal("incident ID should be set")
	}

	got, updates, err := GetIncident(incident.ID)
	if err != nil {
		t.Fatalf("GetIncident failed: %v", err)
	}
	if got.Title != "Chat API degraded" {
		t.Fatalf("title = %s", got.Title)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(updates))
	}
}

func TestAddIncidentUpdate(t *testing.T) {
	newTestDB(t)
	incident := &ServiceIncident{Title: "Test incident", Severity: IncidentSeverityCritical}
	CreateIncident(incident)

	update := &ServiceIncidentUpdate{
		IncidentID: incident.ID,
		Status:     IncidentStatusIdentified,
		Message:    "Root cause identified",
	}
	if err := AddIncidentUpdate(update); err != nil {
		t.Fatalf("AddIncidentUpdate failed: %v", err)
	}

	_, updates, _ := GetIncident(incident.ID)
	if len(updates) != 1 || updates[0].Message != "Root cause identified" {
		t.Fatalf("unexpected updates: %+v", updates)
	}

	// Verify incident status was updated.
	incident2, _, _ := GetIncident(incident.ID)
	if incident2.Status != IncidentStatusIdentified {
		t.Fatalf("incident status should be identified, got %s", incident2.Status)
	}
}

func TestResolveIncidentSetsResolvedAt(t *testing.T) {
	newTestDB(t)
	incident := &ServiceIncident{Title: "Resolved test"}
	CreateIncident(incident)

	AddIncidentUpdate(&ServiceIncidentUpdate{
		IncidentID: incident.ID,
		Status:     IncidentStatusResolved,
		Message:    "Fixed",
	})

	got, _, _ := GetIncident(incident.ID)
	if got.Status != IncidentStatusResolved {
		t.Fatalf("expected resolved, got %s", got.Status)
	}
	if got.ResolvedAt == nil || *got.ResolvedAt == 0 {
		t.Fatal("ResolvedAt should be set")
	}
}

func TestListPublicIncidents(t *testing.T) {
	newTestDB(t)
	// Active incident
	CreateIncident(&ServiceIncident{Title: "Active", Severity: IncidentSeverityWarning, Status: IncidentStatusInvestigating})
	// Resolved recently
	inc2 := &ServiceIncident{Title: "Recent resolved", Severity: IncidentSeverityInfo, Status: IncidentStatusResolved}
	CreateIncident(inc2)
	now := time.Now().Unix()
	DB.Model(&ServiceIncident{}).Where("id = ?", inc2.ID).Update("resolved_at", now)

	// Old resolved (should not appear)
	inc3 := &ServiceIncident{Title: "Old resolved", Severity: IncidentSeverityInfo, Status: IncidentStatusResolved}
	CreateIncident(inc3)
	DB.Model(&ServiceIncident{}).Where("id = ?", inc3.ID).Update("resolved_at", now-8*24*3600)

	list, err := ListPublicIncidents()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 public incidents (active + recent resolved), got %d", len(list))
	}
}

func TestCreateIncidentValidation(t *testing.T) {
	newTestDB(t)
	if err := CreateIncident(&ServiceIncident{Title: ""}); err == nil {
		t.Fatal("empty title should be rejected")
	}
}

func TestGetIncidentNotFound(t *testing.T) {
	newTestDB(t)
	_, _, err := GetIncident(9999)
	if err != ErrIncidentNotFound {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}
