// Package observability holds content-free telemetry shared by the gateway and workers.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"regexp"
	"sync"
	"time"
)

type contextKey struct{}

type Step struct {
	ID          string `json:"id" gorm:"primaryKey;size:32"`
	TraceID     string `json:"trace_id" gorm:"index;size:64"`
	Stage       string `json:"stage"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     int64  `json:"ended_at"`
	Status      string `json:"status"`
	Code        string `json:"code,omitempty"`
	ChannelID   int64  `json:"channel_id,omitempty"`
	UpstreamID  string `json:"upstream_id,omitempty"`
	ReferenceID int64  `json:"reference_id,omitempty"`
}

type Request struct {
	ID           string `json:"trace_id" gorm:"primaryKey;size:64"`
	GatewayID    string `json:"gateway_id" gorm:"index"`
	UserID       int64  `json:"user_id" gorm:"index"`
	Model        string `json:"model" gorm:"size:128"`
	Protocol     string `json:"protocol"`
	Vendor       string `json:"vendor"`
	ServiceGroup string `json:"service_group"`
	StartedAt    int64  `json:"started_at" gorm:"index"`
	EndedAt      int64  `json:"ended_at" gorm:"index"`
	Status       int    `json:"status"`
	DurationMS   int64  `json:"duration_ms"`
	GatewayMS    int64  `json:"gateway_ms"`
	TTFTMS       int64  `json:"ttft_ms"`
	Cancelled    bool   `json:"cancelled"`
	Failed       bool   `json:"failed"`
}

type Trace struct {
	mu      sync.Mutex
	Request Request
	steps   []Step
}

func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}

func Start(ctx context.Context, gateway, protocol string) (context.Context, *Trace) {
	t := &Trace{Request: Request{ID: NewID() + NewID(), GatewayID: gateway, Protocol: protocol, StartedAt: time.Now().UnixMilli()}}
	return context.WithValue(ctx, contextKey{}, t), t
}

func From(ctx context.Context) *Trace { t, _ := ctx.Value(contextKey{}).(*Trace); return t }
func ID(ctx context.Context) string {
	if t := From(ctx); t != nil {
		return t.Request.ID
	}
	return ""
}

func (t *Trace) Usage(ttft int64, failed, cancelled bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Request.TTFTMS, t.Request.Failed, t.Request.Cancelled = ttft, failed, cancelled
}

// Labels contain identifiers only. Payloads, arbitrary errors, headers and URLs are never accepted.
func (t *Trace) Labels(user int64, model, group, vendor string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if user > 0 {
		t.Request.UserID = user
	}
	if model != "" {
		t.Request.Model = bounded(model, 128)
	}
	if group != "" {
		t.Request.ServiceGroup = bounded(group, 128)
	}
	if vendor != "" {
		t.Request.Vendor = bounded(vendor, 128)
	}
}

func bounded(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

var safeCode = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)

func SafeCode(s string) string {
	if safeCode.MatchString(s) {
		return s
	}
	return ""
}

func Begin(ctx context.Context, stage string, channel int64) func(string, string) {
	start := time.Now().UnixMilli()
	return func(status, code string) {
		if t := From(ctx); t != nil {
			t.Add(Step{Stage: stage, StartedAt: start, EndedAt: time.Now().UnixMilli(), Status: status, Code: SafeCode(code), ChannelID: channel})
		}
	}
}

func (t *Trace) Add(step Step) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.steps) >= 256 {
		return
	}
	step.ID, step.TraceID = NewID(), t.Request.ID
	step.Code, step.UpstreamID = SafeCode(step.Code), SafeCode(step.UpstreamID)
	t.steps = append(t.steps, step)
	Log(step.TraceID, step.Stage, step.Status)
}

func (t *Trace) Finish(status int, cancelled bool) (Request, []Step) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.Request
	r.EndedAt, r.Status, r.Cancelled = time.Now().UnixMilli(), status, cancelled || r.Cancelled
	r.DurationMS = max(0, r.EndedAt-r.StartedAt)
	var upstream int64
	for _, s := range t.steps {
		if s.Stage == "upstream" {
			upstream += max(0, s.EndedAt-s.StartedAt)
		}
	}
	r.GatewayMS = max(0, r.DurationMS-upstream)
	return r, append([]Step(nil), t.steps...)
}

func Log(trace, event, status string) {
	if trace == "" {
		trace = NewID()
	}
	raw, _ := json.Marshal(map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "trace_id": trace, "event": SafeCode(event), "status": SafeCode(status)})
	log.New(log.Writer(), "", 0).Print(string(raw))
}
