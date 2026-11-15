package service

import (
	"context"
	"log"
	"sync"
	"time"

	"starhub/model"
)

var securityDeliveryMu sync.Mutex

func securityMessage(action, language string) (string, string) {
	if action == "email_changed" {
		switch language {
		case "zh":
			return "StarHub 安全通知", "您的账户邮箱已更改。如果并非本人操作，请联系管理员。"
		case "ar":
			return "إشعار أمان StarHub", "تم تغيير بريد حسابك. اتصل بالمسؤول إذا لم تقم بهذا التغيير."
		case "ko":
			return "StarHub 보안 알림", "계정 이메일이 변경되었습니다. 본인이 변경하지 않았다면 관리자에게 문의하세요."
		default:
			return "StarHub security notification", "Your account email was changed. Contact your administrator if you did not make this change."
		}
	}
	switch language {
	case "zh":
		return "StarHub 安全通知", "您的账户密码已更改。如果并非本人操作，请联系管理员。"
	case "ar":
		return "إشعار أمان StarHub", "تم تغيير كلمة مرور حسابك. اتصل بالمسؤول إذا لم تقم بهذا التغيير."
	case "ko":
		return "StarHub 보안 알림", "계정 비밀번호가 변경되었습니다. 본인이 변경하지 않았다면 관리자에게 문의하세요."
	default:
		return "StarHub security notification", "Your account password was changed. Contact your administrator if you did not make this change."
	}
}

// DeliverSecurityNotificationsOnce is serialized for the supported single instance.
// A stable ID lets bridges deduplicate retries after ambiguous acknowledgments.
func DeliverSecurityNotificationsOnce(ctx context.Context) error {
	securityDeliveryMu.Lock()
	defer securityDeliveryMu.Unlock()
	if !IsEmailEnabled() {
		return nil
	}
	var rows []model.SecurityNotification
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := model.DB.WithContext(queryCtx).Where("status = ? AND next_attempt <= ?", "pending", time.Now().Unix()).Order("next_attempt, id").Limit(32).Find(&rows).Error
	cancel()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		subject, body := securityMessage(row.Action, row.Language)
		emailMu.RLock()
		provider := emailProvider
		emailMu.RUnlock()
		var err error
		if bridge, ok := provider.(*messageBridge); ok {
			err = bridge.sendContext(ctx, map[string]string{"kind": "security_notification", "id": row.ID, "to": row.Email, "subject": subject, "body": body})
		} else {
			err = provider.SendNotification(row.Email, subject, body)
		}
		if completeErr := model.CompleteSecurityNotificationContext(ctx, row, err == nil); completeErr != nil {
			return completeErr
		}
		if err != nil && row.Attempts == 7 {
			deliverAlert(AlertEvent{Type: "security_notification_failed", Severity: "warning", Message: "Security notification exhausted delivery attempts; inspect provider acceptance history", Timestamp: time.Now().Unix()})
		}
	}
	return nil
}

func RunSecurityNotifications(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := DeliverSecurityNotificationsOnce(ctx); err != nil && ctx.Err() == nil {
			log.Print("security notification processing failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
