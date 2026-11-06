package model

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnnouncementText struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
	AR string `json:"ar"`
}

type ServiceAnnouncement struct {
	ID             int64            `gorm:"primaryKey" json:"id"`
	Version        int64            `json:"version"`
	Title          AnnouncementText `gorm:"serializer:json" json:"title"`
	Content        AnnouncementText `gorm:"serializer:json" json:"content"`
	Vendors        []string         `gorm:"serializer:json" json:"vendors"`
	Models         []string         `gorm:"serializer:json" json:"models"`
	Status         string           `gorm:"index" json:"status"`
	CreatedAt      int64            `json:"created_at"`
	UpdatedAt      int64            `gorm:"index" json:"updated_at"`
	ResolvedAt     int64            `json:"resolved_at"`
	CreationKey    string           `gorm:"uniqueIndex" json:"-"`
	CreationDigest string           `json:"-"`
}

type AnnouncementWrite struct {
	ExpectedVersion int64            `json:"expected_version"`
	OperationID     string           `json:"operation_id"`
	Title           AnnouncementText `json:"title"`
	Content         AnnouncementText `json:"content"`
	Vendors         []string         `json:"vendors"`
	Models          []string         `json:"models"`
	Status          string           `json:"status"`
}

func announcementString(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return "", false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", false
		}
	}
	return value, true
}

func validateAnnouncement(input *AnnouncementWrite) error {
	for _, field := range []struct {
		text  *AnnouncementText
		limit int
	}{{&input.Title, 200}, {&input.Content, 5000}} {
		for _, value := range []*string{&field.text.ZH, &field.text.EN, &field.text.AR} {
			trimmed, ok := announcementString(*value, field.limit)
			if !ok {
				return ErrAdminInvalid
			}
			*value = trimmed
		}
	}
	if !slices.Contains([]string{"investigating", "identified", "monitoring", "resolved"}, input.Status) {
		return ErrAdminInvalid
	}
	if len(input.Vendors)+len(input.Models) == 0 || len(input.Vendors)+len(input.Models) > 40 {
		return ErrAdminInvalid
	}
	for _, list := range []*[]string{&input.Vendors, &input.Models} {
		clean := []string{}
		for _, name := range *list {
			name, ok := announcementString(name, 128)
			if !ok || strings.ContainsAny(name, "\n\t") {
				return ErrAdminInvalid
			}
			clean = append(clean, name)
		}
		slices.Sort(clean)
		*list = slices.Compact(clean)
	}
	return nil
}

func SaveServiceAnnouncement(ctx context.Context, id int64, input AnnouncementWrite, actor int64, ip string) (*ServiceAnnouncement, error) {
	if actor <= 0 || id < 0 || input.ExpectedVersion < 0 || input.ExpectedVersion >= maxResourceRevision {
		return nil, ErrAdminInvalid
	}
	if err := validateAnnouncement(&input); err != nil {
		return nil, err
	}
	if id == 0 && (input.ExpectedVersion != 0 || input.Status == "resolved" || !resourceText(input.OperationID, 128, true)) {
		return nil, ErrAdminInvalid
	}
	now := time.Now().Unix()
	row := ServiceAnnouncement{Version: 1, Title: input.Title, Content: input.Content, Vendors: input.Vendors, Models: input.Models, Status: input.Status, CreatedAt: now, UpdatedAt: now}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before *ServiceAnnouncement
		action := "announcement_published"
		if id == 0 {
			row.CreationKey = monitorHash(actor, input.OperationID)
			row.CreationDigest = monitorHash(input.Title, input.Content, input.Vendors, input.Models, input.Status)
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected == 0 {
				var previous ServiceAnnouncement
				if err := tx.First(&previous, "creation_key = ?", row.CreationKey).Error; err != nil {
					return err
				}
				if previous.CreationDigest != row.CreationDigest {
					return ErrAdminConflict
				}
				row = previous
				return nil
			}
		} else {
			var previous ServiceAnnouncement
			// Acquire the SQLite writer before reading the version and audit preimage.
			lock := tx.Model(&ServiceAnnouncement{}).Where("id = ?", id).UpdateColumn("version", gorm.Expr("version"))
			if lock.Error != nil {
				return lock.Error
			}
			if err := tx.First(&previous, id).Error; err != nil {
				return err
			}
			if previous.Version != input.ExpectedVersion || (previous.Status == "resolved" && input.Status != "resolved") {
				return ErrAdminConflict
			}
			before = &previous
			row.ID, row.Version, row.CreatedAt = id, previous.Version+1, previous.CreatedAt
			row.CreationKey, row.CreationDigest, row.ResolvedAt = previous.CreationKey, previous.CreationDigest, previous.ResolvedAt
			action = "announcement_updated"
			if row.Status == "resolved" && previous.Status != "resolved" {
				row.ResolvedAt = now
				action = "announcement_resolved"
			}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return RecordAdminChange(tx, actor, action, "announcements", strconv.FormatInt(row.ID, 10), before, &row, "", ip)
	})
	return &row, err
}

type AnnouncementList struct {
	Items []ServiceAnnouncement `json:"items"`
	Total int64                 `json:"total"`
}

func ListServiceAnnouncements(ctx context.Context, status string, page int, public bool) (*AnnouncementList, error) {
	if page < 1 || page > 100000 || !slices.Contains([]string{"all", "active", "resolved"}, status) {
		return nil, ErrAdminInvalid
	}
	result := &AnnouncementList{Items: []ServiceAnnouncement{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&ServiceAnnouncement{})
		if public || status == "active" {
			q = q.Where("status <> ?", "resolved")
		} else if status == "resolved" {
			q = q.Where("status = ?", "resolved")
		}
		if err := q.Count(&result.Total).Error; err != nil {
			return err
		}
		q = q.Order("updated_at DESC, id DESC")
		if !public {
			q = q.Offset((page - 1) * 20).Limit(20)
		}
		return q.Find(&result.Items).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return result, err
}
