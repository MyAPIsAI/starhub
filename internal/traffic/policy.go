// Package traffic owns model/group traffic allocation independently of channel defaults.
package traffic

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

const OptionKey = "channel_traffic_allocation"
const MaxWeight int64 = 1000000
const MaxPriority int64 = 1000000

type ChannelWeight struct {
	ChannelID int64  `json:"channel_id,string"`
	Weight    int64  `json:"weight"`
	Priority  *int64 `json:"priority,omitempty"`
}

type Rule struct {
	Group    string          `json:"group"`
	Model    string          `json:"model"`
	Enabled  bool            `json:"enabled"`
	Channels []ChannelWeight `json:"channels"`
}

type Policy struct {
	Rules []Rule `json:"rules"`
}

var current atomic.Pointer[Policy]
var empty = &Policy{Rules: []Rule{}}

// Snapshot is immutable after publication and may be retained for an entire request.
func Snapshot() *Policy {
	if p := current.Load(); p != nil {
		return p
	}
	return empty
}

func (p *Policy) Lookup(group, model string) *Rule {
	for i := range p.Rules {
		r := &p.Rules[i]
		if r.Enabled && r.Group == group && r.Model == model {
			return r
		}
	}
	return nil
}

func (r *Rule) Weight(id int64) int64 {
	for _, ch := range r.Channels {
		if ch.ChannelID == id {
			return ch.Weight
		}
	}
	return 0
}

// Omitted priorities preserve the channel defaults used by older saved rules.
func (r *Rule) Priority(id, fallback int64) int64 {
	for _, ch := range r.Channels {
		if ch.ChannelID == id && ch.Priority != nil {
			return *ch.Priority
		}
	}
	return fallback
}

func ValidScope(group, model string) bool {
	return group != "" && group != "auto" && len(group) <= 64 && strings.TrimSpace(group) == group &&
		model != "" && len(model) <= 128 && strings.TrimSpace(model) == model &&
		!strings.ContainsAny(group+model, "\x00\r\n")
}

func Decode(value string) (*Policy, error) {
	if len(value) > 256*1024 {
		return nil, fmt.Errorf("traffic configuration is too large")
	}
	var p Policy
	d := json.NewDecoder(strings.NewReader(value))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing traffic configuration")
	}
	if p.Rules == nil || len(p.Rules) > 100 {
		return nil, fmt.Errorf("traffic rules must be an array of at most 100 scopes")
	}
	seen := map[[2]string]bool{}
	for i := range p.Rules {
		r := &p.Rules[i]
		key := [2]string{r.Group, r.Model}
		if !ValidScope(r.Group, r.Model) || seen[key] || len(r.Channels) > 500 || r.Channels == nil {
			return nil, fmt.Errorf("invalid or duplicate traffic scope")
		}
		seen[key] = true
		ids, total := map[int64]bool{}, int64(0)
		for _, ch := range r.Channels {
			if ch.Priority != nil && (*ch.Priority < -MaxPriority || *ch.Priority > MaxPriority) {
				return nil, fmt.Errorf("channel priority must be an integer between -1000000 and 1000000")
			}
			if ch.ChannelID <= 0 || ids[ch.ChannelID] || ch.Weight < 0 || ch.Weight > MaxWeight {
				return nil, fmt.Errorf("invalid channel or weight: %s", strconv.FormatInt(ch.ChannelID, 10))
			}
			ids[ch.ChannelID] = true
			total += ch.Weight
		}
		if r.Enabled && total == 0 {
			return nil, fmt.Errorf("enabled traffic scopes need a positive weight")
		}
		sort.Slice(r.Channels, func(i, j int) bool { return r.Channels[i].ChannelID < r.Channels[j].ChannelID })
	}
	sort.Slice(p.Rules, func(i, j int) bool {
		if p.Rules[i].Group == p.Rules[j].Group {
			return p.Rules[i].Model < p.Rules[j].Model
		}
		return p.Rules[i].Group < p.Rules[j].Group
	})
	return &p, nil
}

func (p *Policy) JSON() string { b, _ := json.Marshal(p); return string(b) }

func Normalize(value string) (string, error) {
	p, err := Decode(value)
	if err != nil {
		return "", err
	}
	return p.JSON(), nil
}

func Apply(value string) error {
	p, err := Decode(value)
	if err != nil {
		return err
	}
	current.Store(p)
	return nil
}
