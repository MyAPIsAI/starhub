package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrIncidentNotFound = errors.New("service incident not found")

const (
	IncidentSeverityInfo        = "info"
	IncidentSeverityWarning     = "warning"
	IncidentSeverityCritical    = "critical"
	IncidentSeverityMaintenance = "maintenance"

	IncidentStatusInvestigating = "investigating"
	IncidentStatusIdentified    = "identified"
	IncidentStatusMonitoring    = "monitoring"
	IncidentStatusResolved      = "resolved"
)

// ServiceIncident records a service event visible on the public status page.
type ServiceIncident struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Title       string `gorm:"size:256;not null" json:"title"`
	Description string `gorm:"type:text;not null;default:''" json:"description"`
	Severity    string `gorm:"size:32;not null;default:'info'" json:"severity"`
	Status      string `gorm:"size:32;not null;default:'investigating'" json:"status"`
	Component   string `gorm:"size:64;not null;default:''" json:"component"`
	CreatedBy   int64  `gorm:"not null;default:0" json:"created_by"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   int64  `gorm:"autoUpdateTime" json:"updated_at"`
	ResolvedAt  *int64 `json:"resolved_at,omitempty"`
}

// ServiceIncidentUpdate records a status update on an incident.
type ServiceIncidentUpdate struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID int64  `gorm:"index;not null" json:"incident_id"`
	Status     string `gorm:"size:32;not null" json:"status"`
	Message    string `gorm:"type:text;not null;default:''" json:"message"`
	CreatedBy  int64  `gorm:"not null;default:0" json:"created_by"`
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
}

// CreateIncident creates a new service incident.
func CreateIncident(incident *ServiceIncident) error {
	if strings.TrimSpace(incident.Title) == "" {
		return errors.New("incident title is required")
	}
	if incident.Severity == "" {
		incident.Severity = IncidentSeverityInfo
	}
	if incident.Status == "" {
		incident.Status = IncidentStatusInvestigating
	}
	return DB.Create(incident).Error
}

// AddIncidentUpdate adds a status update to an incident and updates its
// current status. When the status is "resolved", ResolvedAt is set.
func AddIncidentUpdate(update *ServiceIncidentUpdate) error {
	if update.IncidentID <= 0 {
		return errors.New("incident_id is required")
	}
	if update.Status == "" {
		return errors.New("status is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var incident ServiceIncident
		if err := tx.First(&incident, "id = ?", update.IncidentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrIncidentNotFound
			}
			return err
		}
		if err := tx.Create(update).Error; err != nil {
			return err
		}
		updates := map[string]any{"status": update.Status}
		if update.Status == IncidentStatusResolved {
			now := time.Now().Unix()
			updates["resolved_at"] = now
		}
		return tx.Model(&ServiceIncident{}).Where("id = ?", update.IncidentID).
			Updates(updates).Error
	})
}

// GetIncident returns an incident with its updates.
func GetIncident(id int64) (*ServiceIncident, []ServiceIncidentUpdate, error) {
	var incident ServiceIncident
	err := DB.First(&incident, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrIncidentNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	var updates []ServiceIncidentUpdate
	DB.Where("incident_id = ?", id).Order("id ASC").Find(&updates)
	return &incident, updates, nil
}

// ListIncidents returns all incidents, newest first.
func ListIncidents(limit int) ([]ServiceIncident, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var incidents []ServiceIncident
	err := DB.Order("id DESC").Limit(limit).Find(&incidents).Error
	return incidents, err
}

// ListPublicIncidents returns active incidents plus recently resolved ones.
// Active = status != resolved. Recently resolved = resolved within 7 days.
func ListPublicIncidents() ([]ServiceIncident, error) {
	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
	var incidents []ServiceIncident
	err := DB.Where("status != ? OR (resolved_at IS NOT NULL AND resolved_at > ?)",
		IncidentStatusResolved, weekAgo).
		Order("id DESC").Limit(50).Find(&incidents).Error
	return incidents, err
}
