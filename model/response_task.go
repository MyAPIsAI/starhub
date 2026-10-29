package model

import (
	"encoding/json"
	"gorm.io/gorm"
)

// BackgroundResponseOutcome stores accounting metadata only, never response text.
type BackgroundResponseOutcome struct {
	UsageSource      string `json:"usage_source,omitempty"`
	ResponseStatus   string `json:"response_status"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	HasImages        bool   `json:"has_images"`
	HasTools         bool   `json:"has_tools"`
	HasReasoning     bool   `json:"has_reasoning"`
}

func updateBackgroundResponseReference(tx *gorm.DB, task *Task) error {
	if task.Platform != "responses" {
		return nil
	}
	var result BackgroundResponseOutcome
	if err := json.Unmarshal([]byte(task.PendingResultMeta), &result); err != nil {
		return err
	}
	if err := tx.Model(&ResponseReference{}).Where("task_id = ? AND user_id = ? AND token_id = ?", task.ID, task.UserID, task.TokenID).Updates(map[string]any{"status": result.ResponseStatus, "prompt_tokens": result.PromptTokens, "completion_tokens": result.CompletionTokens, "has_images": result.HasImages, "has_tools": result.HasTools, "has_reasoning": result.HasReasoning}).Error; err != nil {
		return err
	}
	var snapshot struct {
		Binding struct{ ConversationID, ConversationNonce string } `json:"binding"`
	}
	if err := json.Unmarshal([]byte(task.RequestMeta), &snapshot); err != nil {
		return err
	}
	responseID := ""
	if result.ResponseStatus == "completed" || result.ResponseStatus == "incomplete" {
		responseID = task.TaskID
	}
	return finishResponseConversation(tx, snapshot.Binding.ConversationID, snapshot.Binding.ConversationNonce, responseID)
}
func BindBackgroundResponseTask(id int64, providerID string) error {
	return DB.Model(&Task{}).Where("id = ? AND task_id = '' AND status = ?", id, "submitting").Updates(map[string]any{"task_id": providerID, "status": "submitted"}).Error
}
func QuarantineResponseID(account, id string) error {
	return DB.Model(&ResponseReference{}).Where("account_identity = ? AND response_id = ?", account, id).Update("conflicted", true).Error
}
