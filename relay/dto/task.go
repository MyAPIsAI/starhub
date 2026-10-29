package dto

import (
	"encoding/json"
	"fmt"
)

// Task statuses (submit-and-poll lifecycle, P3.3).
const (
	TaskStatusSubmitted  = "submitted"
	TaskStatusInProgress = "in_progress"
	TaskStatusSucceeded  = "succeeded"
	TaskStatusFailed     = "failed"
	TaskStatusCancelled  = "cancelled"
)

// IsTerminalTaskStatus reports whether the status ends the lifecycle (and
// therefore triggers the final settlement).
func IsTerminalTaskStatus(status string) bool {
	switch status {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// Task actions supported by the gateway.
const (
	TaskActionVideo = "video"
)

// Bounds for the Sora-style video task request. Multipliers are validated
// before the pre-charge so a hostile seconds/n combination cannot inflate
// the estimate.
const (
	MaxTaskSeconds = 60
	MaxTaskN       = 10
)

// TaskRequest is the normalized submit request of a task platform. It is
// persisted with the task so later polls can re-price the final bill.
type TaskRequest struct {
	Action string `json:"action"`
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	// Video fields (action == "video").
	Seconds int                        `json:"seconds,omitempty"`
	Size    string                     `json:"size,omitempty"`
	N       int                        `json:"n,omitempty"`
	Extra   map[string]json.RawMessage `json:"extra,omitempty"`
}

// ParseVideoTaskRequest parses and validates a Sora-style video submit body
// ({model, prompt, seconds, size, n}).
func ParseVideoTaskRequest(body []byte) (*TaskRequest, error) {
	var raw struct {
		Model   string                     `json:"model"`
		Prompt  string                     `json:"prompt"`
		Seconds *int                       `json:"seconds"`
		Size    string                     `json:"size"`
		N       *int                       `json:"n"`
		Extra   map[string]json.RawMessage `json:"extra"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	req := &TaskRequest{Action: TaskActionVideo, Model: raw.Model, Prompt: raw.Prompt, Size: raw.Size, Extra: raw.Extra}
	if raw.Seconds != nil {
		req.Seconds = *raw.Seconds
	} else {
		req.Seconds = 4
	}
	if raw.N != nil {
		req.N = *raw.N
	} else {
		req.N = 1
	}
	if req.Model == "" {
		return nil, fmt.Errorf("the \"model\" field is required")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("the \"prompt\" field is required")
	}
	if req.Seconds <= 0 || req.Seconds > MaxTaskSeconds {
		return nil, fmt.Errorf("seconds must be between 1 and %d, got %d", MaxTaskSeconds, req.Seconds)
	}
	if req.N <= 0 || req.N > MaxTaskN {
		return nil, fmt.Errorf("n must be between 1 and %d, got %d", MaxTaskN, req.N)
	}
	if req.Size != "" && !sizePattern.MatchString(req.Size) && req.Size != "auto" {
		return nil, fmt.Errorf("invalid size %q: expected <width>x<height>", req.Size)
	}
	return req, nil
}

// TaskResult is the normalized result of one upstream poll.
type TaskResult struct {
	Status     string   `json:"status"` // submitted / in_progress / succeeded / failed / cancelled
	Progress   int      `json:"progress"`
	FailReason string   `json:"fail_reason,omitempty"`
	Outputs    []string `json:"outputs,omitempty"` // video / music URLs
	Usage      *Usage   `json:"usage,omitempty"`
}

// TaskSubmitResponse is the client-facing submit response.
type TaskSubmitResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

// TaskView is the client-facing task status document.
type TaskView struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Progress     int      `json:"progress"`
	VideoURL     string   `json:"video_url,omitempty"`
	Outputs      []string `json:"outputs,omitempty"`
	FailReason   string   `json:"fail_reason,omitempty"`
	Model        string   `json:"model,omitempty"`
	SubmitTime   int64    `json:"submit_time,omitempty"`
	CompleteTime int64    `json:"complete_time,omitempty"`
}

// ParseVideoTaskSubmitResponse extracts the upstream task id from an OpenAI
// Sora-style submit response ({"id": "...", "status": "queued", ...}).
func ParseVideoTaskSubmitResponse(body []byte) (string, error) {
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		if resp.Error != nil && resp.Error.Message != "" {
			return "", fmt.Errorf("upstream: %s", resp.Error.Message)
		}
		return "", fmt.Errorf("upstream response has no task id")
	}
	return resp.ID, nil
}

// ParseVideoTaskResult normalizes an OpenAI Sora-style poll response.
func ParseVideoTaskResult(body []byte) (*TaskResult, error) {
	var resp struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Error    *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Video *struct {
			URL string `json:"url"`
		} `json:"video"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out := &TaskResult{Progress: resp.Progress}
	switch resp.Status {
	case "queued", "pending":
		out.Status = TaskStatusInProgress
	case "in_progress", "processing":
		out.Status = TaskStatusInProgress
	case "completed", "succeeded":
		out.Status = TaskStatusSucceeded
	case "failed", "error":
		out.Status = TaskStatusFailed
	case "cancelled", "canceled":
		out.Status = TaskStatusCancelled
	default:
		out.Status = TaskStatusInProgress
	}
	if resp.Error != nil {
		out.FailReason = resp.Error.Message
		if out.Status != TaskStatusSucceeded {
			out.Status = TaskStatusFailed
		}
	}
	if resp.Video != nil && resp.Video.URL != "" {
		out.Outputs = append(out.Outputs, resp.Video.URL)
	}
	return out, nil
}
