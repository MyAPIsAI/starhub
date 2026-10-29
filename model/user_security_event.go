package model

// UserSecurityEvent records only successful guarded profile changes. It never
// stores old/new contacts, password material, cookies or client-supplied text.
type UserSecurityEvent struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64  `gorm:"index:idx_user_security_owner_id,priority:1;not null" json:"-"`
	Action    string `gorm:"size:32;not null" json:"action"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

func ListUserSecurityEvents(owner int64) ([]UserSecurityEvent, error) {
	events := []UserSecurityEvent{}
	err := DB.Where("user_id = ?", owner).Order("id DESC").Limit(5).Find(&events).Error
	return events, err
}
