// Package authz implements a lightweight, policy-based authorization layer
// for the management API. It is a Casbin-style RBAC without the Casbin
// dependency: policies are defined in code (role -> resource -> action ->
// allow/deny), and the middleware checks the request against the policy.
//
// The four roles from common map to policy sets:
//
//	Root  — every admin action plus user-role management (promote/demote).
//	Admin — channel/token/option/audit management (the day-to-day ops set).
//	User  — own-token management only (create/list/update/delete own tokens).
//	Finance — read-only financial summaries plus own-token/log service.
//
// A deny entry wins over an allow entry (deny-by-default for unlisted
// resources). The policy is static and compiled at init time; runtime
// changes go through code, not the database.
package authz

import (
	"net/http"
	"strings"

	"starhub/common"
)

// Action is the HTTP method mapped to a policy action.
type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
)

// Resource is the management API resource area.
type Resource string

const (
	ResourceUser         Resource = "user"
	ResourceChannel      Resource = "channel"
	ResourceToken        Resource = "token"
	ResourceOption       Resource = "option"
	ResourceLog          Resource = "log"
	ResourceAudit        Resource = "audit"
	ResourceRedemption   Resource = "redemption"
	ResourceFinance      Resource = "finance"
	ResourceInventory    Resource = "inventory"
	ResourceRegion       Resource = "region"
	ResourceServiceGroup Resource = "service_group"
	ResourceModelMeta    Resource = "model_metadata"
)

// policy maps role -> resource -> allowed actions. An empty action set means
// "all actions allowed" (Root on most resources). Missing entries are
// denied.
var policy = map[int]map[Resource]map[Action]bool{
	common.RoleRoot: {
		ResourceUser:         {}, // all actions
		ResourceChannel:      {}, // all actions
		ResourceToken:        {}, // all actions
		ResourceOption:       {}, // all actions
		ResourceLog:          {}, // all actions
		ResourceAudit:        {}, // all actions
		ResourceRedemption:   {}, // all actions
		ResourceFinance:      {ActionRead: true},
		ResourceInventory:    {ActionRead: true, ActionWrite: true},
		ResourceRegion:       {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceServiceGroup: {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceModelMeta:    {ActionRead: true, ActionWrite: true, ActionDelete: true},
	},
	common.RoleAdmin: {
		ResourceUser:         {ActionRead: true},
		ResourceChannel:      {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceToken:        {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceOption:       {ActionRead: true, ActionWrite: true},
		ResourceLog:          {ActionRead: true},
		ResourceAudit:        {ActionRead: true},
		ResourceRedemption:   {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceFinance:      {ActionRead: true},
		ResourceInventory:    {ActionRead: true, ActionWrite: true},
		ResourceRegion:       {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceServiceGroup: {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceModelMeta:    {ActionRead: true, ActionWrite: true, ActionDelete: true},
	},
	common.RoleFinance: {
		ResourceFinance: {ActionRead: true},
		ResourceToken:   {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceLog:     {ActionRead: true},
	},
	common.RoleUser: {
		ResourceToken: {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceLog:   {ActionRead: true},
	},
}

// Allow reports whether the role may perform the action on the resource.
// An empty action set in the policy means "all actions allowed" (Root).
func Allow(role int, resource Resource, action Action) bool {
	resources, ok := policy[role]
	if !ok {
		return false
	}
	actions, ok := resources[resource]
	if !ok {
		return false
	}
	if len(actions) == 0 {
		return true // all actions allowed
	}
	return actions[action]
}

// methodAction maps an HTTP method to a policy action.
func methodAction(method string) Action {
	switch method {
	case http.MethodGet:
		return ActionRead
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return ActionWrite
	case http.MethodDelete:
		return ActionDelete
	default:
		return ActionRead
	}
}

// pathResource maps a request path to a policy resource. Returns false when
// the path is not a management API resource (e.g. relay traffic).
func pathResource(path string) (Resource, bool) {
	path = strings.TrimSuffix(path, "/")
	switch {
	case strings.HasPrefix(path, "/api/user"):
		return ResourceUser, true
	case strings.HasPrefix(path, "/api/channel"):
		return ResourceChannel, true
	case strings.HasPrefix(path, "/api/token"):
		return ResourceToken, true
	case strings.HasPrefix(path, "/api/option"):
		return ResourceOption, true
	case strings.HasPrefix(path, "/api/log"):
		return ResourceLog, true
	case strings.HasPrefix(path, "/api/audit"):
		return ResourceAudit, true
	case strings.HasPrefix(path, "/api/redemption"):
		return ResourceRedemption, true
	case path == "/api/finance" || strings.HasPrefix(path, "/api/finance/"):
		return ResourceFinance, true
	case path == "/api/resource" || strings.HasPrefix(path, "/api/resource/"), path == "/api/placement" || strings.HasPrefix(path, "/api/placement/"):
		return ResourceInventory, true
	case path == "/api/region" || strings.HasPrefix(path, "/api/region/"):
		return ResourceRegion, true
	case path == "/api/service-group" || strings.HasPrefix(path, "/api/service-group/"):
		return ResourceServiceGroup, true
	case path == "/api/plaza/meta" || strings.HasPrefix(path, "/api/plaza/meta/"):
		return ResourceModelMeta, true
	}
	return "", false
}

// CheckRequest evaluates the request against the policy. It returns true
// when the caller's role may perform the action on the resource. Relay
// traffic (non-/api paths) is always allowed (the relay chain enforces its
// own auth).
func CheckRequest(role int, r *http.Request) bool {
	resource, ok := pathResource(r.URL.Path)
	if !ok {
		return true // not a management API resource
	}
	return Allow(role, resource, methodAction(r.Method))
}
