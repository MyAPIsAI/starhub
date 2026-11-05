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
	ResourceUser            Resource = "user"
	ResourceChannel         Resource = "channel"
	ResourceTaskPlugin      Resource = "task_plugin"
	ResourceToken           Resource = "token"
	ResourceOption          Resource = "option"
	ResourceLog             Resource = "log"
	ResourceAudit           Resource = "audit"
	ResourceRedemption      Resource = "redemption"
	ResourceFinance         Resource = "finance"
	ResourceInventory       Resource = "inventory"
	ResourceRegion          Resource = "region"
	ResourceServiceGroup    Resource = "service_group"
	ResourceModelMeta       Resource = "model_metadata"
	ResourceStaff           Resource = "staff_accounts"
	ResourceSync            Resource = "resource_sync"
	ResourceAdjustment      Resource = "financial_adjustment"
	ResourceBilling         Resource = "billing"
	ResourceExport          Resource = "export"
	ResourceMetrics         Resource = "metrics"
	ResourceSystem          Resource = "system"
	ResourceCustomerControl Resource = "customer_control"
	ResourceReconciliation  Resource = "reconciliation"
)

// policy maps role -> resource -> allowed actions. An empty action set means
// "all actions allowed" (Root on most resources). Missing entries are
// denied.
var policy = map[int]map[Resource]map[Action]bool{
	common.RoleRoot: {
		ResourceReconciliation: {},
		ResourceStaff:          {}, ResourceSync: {}, ResourceAdjustment: {}, ResourceBilling: {}, ResourceExport: {}, ResourceMetrics: {}, ResourceSystem: {}, ResourceCustomerControl: {},
		ResourceUser:         {}, // all actions
		ResourceChannel:      {}, // all actions
		ResourceTaskPlugin:   {ActionRead: true, ActionWrite: true},
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
		ResourceUser:      {ActionRead: true},
		ResourceChannel:   {ActionRead: true},
		ResourceToken:     {ActionRead: true},
		ResourceOption:    {ActionRead: true, ActionWrite: true},
		ResourceLog:       {ActionRead: true},
		ResourceAudit:     {ActionRead: true},
		ResourceInventory: {ActionRead: true},
		ResourceExport:    {ActionRead: true, ActionWrite: true}, ResourceMetrics: {ActionRead: true}, ResourceCustomerControl: {ActionRead: true, ActionWrite: true},
		ResourceServiceGroup: {ActionRead: true, ActionWrite: true, ActionDelete: true},
		ResourceModelMeta:    {ActionRead: true, ActionWrite: true, ActionDelete: true},
	},
	common.RoleFinance: {
		ResourceReconciliation:  {ActionRead: true, ActionWrite: true},
		ResourceCustomerControl: {ActionRead: true},
		ResourceUser:            {ActionRead: true}, ResourceAudit: {ActionRead: true}, ResourceBilling: {ActionRead: true}, ResourceAdjustment: {ActionWrite: true}, ResourceExport: {ActionRead: true, ActionWrite: true}, ResourceMetrics: {ActionRead: true},
		ResourceFinance: {ActionRead: true},
	},
	common.RoleTechnical: {
		ResourceCustomerControl: {ActionRead: true},
		ResourceUser:            {ActionRead: true}, ResourceLog: {ActionRead: true}, ResourceAudit: {ActionRead: true}, ResourceChannel: {ActionRead: true, ActionWrite: true, ActionDelete: true}, ResourceInventory: {ActionRead: true, ActionWrite: true}, ResourceRegion: {ActionRead: true}, ResourceSync: {ActionRead: true, ActionWrite: true}, ResourceSystem: {ActionRead: true}, ResourceExport: {ActionRead: true, ActionWrite: true},
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
	case path == "/api/admin/models" || strings.HasPrefix(path, "/api/admin/models/"):
		return ResourceModelMeta, true
	case path == "/api/admin/region-profile" || path == "/api/admin/legal" || strings.HasPrefix(path, "/api/admin/region-profile/"):
		return ResourceStaff, true
	case path == "/api/admin/pricing-history":
		return ResourceModelMeta, true
	case path == "/api/admin/reconciliation":
		return ResourceReconciliation, true
	case path == "/api/admin/customers":
		return ResourceUser, true
	case path == "/api/admin/staff" || strings.HasPrefix(path, "/api/admin/staff/"):
		return ResourceStaff, true
	case path == "/api/admin/resource-sync":
		return ResourceSync, true
	case path == "/api/admin/system":
		return ResourceSystem, true
	case strings.HasPrefix(path, "/api/admin/customers/") && strings.HasSuffix(path, "/adjustments"):
		return ResourceAdjustment, true
	case strings.HasPrefix(path, "/api/admin/customers/"):
		return ResourceCustomerControl, true
	case path == "/api/metrics" || path == "/api/admin/dashboard":
		return ResourceMetrics, true
	case strings.HasPrefix(path, "/api/ledger") || strings.HasPrefix(path, "/api/wallet") || strings.HasPrefix(path, "/api/recharge"):
		return ResourceBilling, true
	case strings.HasPrefix(path, "/api/export"):
		return ResourceExport, true
	case path == "/api/task-plugin" || strings.HasPrefix(path, "/api/task-plugin/"):
		return ResourceTaskPlugin, true
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
	if role == common.RoleAdmin && (r.URL.Path == "/api/option/rollback" || r.URL.Path == "/api/option/history") {
		return false
	}
	resource, ok := pathResource(r.URL.Path)
	if !ok {
		if strings.HasPrefix(r.URL.Path, "/api/admin/") {
			return false
		}
		return true // not a management API resource
	}
	return Allow(role, resource, methodAction(r.Method))
}
