package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrLegalDocumentNotFound = errors.New("legal document not found")

// LegalDocument stores a versioned legal text (user agreement, privacy policy)
// in a specific language and optionally scoped to a region.
type LegalDocument struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Type        string `gorm:"size:32;not null;index" json:"type"` // user_agreement | privacy_policy
	Version     string `gorm:"size:32;not null" json:"version"`
	Language    string `gorm:"size:8;not null;default:'en'" json:"language"`
	Content     string `gorm:"type:text;not null" json:"content"`
	RegionID    string `gorm:"size:64;not null;default:''" json:"region_id"` // empty = global
	PublishedAt int64  `gorm:"not null;default:0" json:"published_at"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

// PublishLegalDocument creates a new version of a legal document. If a
// document with the same (type, version, language, region_id) already exists,
// it is updated. The PublishedAt timestamp is set to now for new documents.
func PublishLegalDocument(doc *LegalDocument) error {
	doc.Type = strings.TrimSpace(doc.Type)
	doc.Version = strings.TrimSpace(doc.Version)
	doc.Language = strings.TrimSpace(doc.Language)
	if doc.Type == "" || doc.Version == "" {
		return errors.New("type and version are required")
	}
	if doc.Language == "" {
		doc.Language = "en"
	}
	if doc.Content == "" {
		return errors.New("content is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing LegalDocument
		err := tx.Where("type = ? AND version = ? AND language = ? AND region_id = ?",
			doc.Type, doc.Version, doc.Language, doc.RegionID).First(&existing).Error
		if err == nil {
			// Update existing.
			existing.Content = doc.Content
			return tx.Save(&existing).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		doc.PublishedAt = time.Now().Unix()
		return tx.Create(doc).Error
	})
}

// GetLegalDocument returns the latest published document of the given type
// in the requested language and region. Falls back to global (empty region)
// if no region-specific document exists.
func GetLegalDocument(docType, language, regionID string) (*LegalDocument, error) {
	// Try region-specific first.
	var doc LegalDocument
	err := DB.Where("type = ? AND language = ? AND region_id = ? AND published_at > 0",
		docType, language, regionID).
		Order("published_at DESC").First(&doc).Error
	if err == nil {
		return &doc, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Fall back to global.
	err = DB.Where("type = ? AND language = ? AND region_id = '' AND published_at > 0",
		docType, language).
		Order("published_at DESC").First(&doc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Fall back to English global.
		err = DB.Where("type = ? AND language = 'en' AND region_id = '' AND published_at > 0",
			docType).
			Order("published_at DESC").First(&doc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLegalDocumentNotFound
		}
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetLatestLegalVersion returns the version string of the latest published
// document of the given type (any language, global scope). Used to check
// whether the user needs to re-consent.
func GetLatestLegalVersion(docType string) (string, error) {
	var doc LegalDocument
	err := DB.Where("type = ? AND published_at > 0", docType).
		Order("id DESC").First(&doc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return doc.Version, nil
}

// ListLegalDocuments returns all legal documents, optionally filtered by type.
func ListLegalDocuments(docType string) ([]LegalDocument, error) {
	var docs []LegalDocument
	q := DB.Model(&LegalDocument{})
	if docType != "" {
		q = q.Where("type = ?", docType)
	}
	err := q.Order("type ASC, published_at DESC").Find(&docs).Error
	return docs, err
}
