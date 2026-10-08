package controller

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
)

// nano-USD per dollar (matches the frontend's 1e9 convention).
const nanoPerUSD = 1_000_000_000

// --- Wallet ---

// GetWallet returns the caller's wallet balance in nano-USD.
func GetWallet(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	wallet, err := model.GetOrCreateWallet(userID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"user_id":    wallet.UserID,
		"balance":    wallet.Balance, // nano-USD
		"created_at": wallet.CreatedAt,
		"updated_at": wallet.UpdatedAt,
	})
}

// --- Ledger ---

// ListLedger returns ledger entries. Admins see all; users see their own.
func ListLedger(w http.ResponseWriter, r *http.Request) {
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
		userID = requestUserID(r)
	}
	// Lazily expire stale orders so the ledger reflects reality.
	_ = model.ExpireRechargeOrders()
	entries, err := model.ListLedger(userID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		data = append(data, map[string]any{
			"id":            e.ID,
			"user_id":       e.UserID,
			"type":          e.Type,
			"amount":        e.Amount,
			"balance_after": e.BalanceAfter,
			"description":   e.Description,
			"order_id":      e.OrderID,
			"trace_id":      e.TraceID,
			"created_at":    e.CreatedAt,
		})
	}
	common.ApiSuccess(w, data)
}

// --- Recharge orders ---

type createRechargeRequest struct {
	AmountUSD string `json:"amount_usd"` // decimal dollars, e.g. "100.00"
	Channel   string `json:"channel"`    // "card" (default)
}

// CreateRechargeOrder creates a pending recharge order. The amount is
// validated server-side: $10–$10,000, at most 2 decimal places.
func CreateRechargeOrder(w http.ResponseWriter, r *http.Request) {
	var req createRechargeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	amountNano, ok := parseRechargeAmount(req.AmountUSD)
	if !ok {
		common.ApiError(w, http.StatusBadRequest, "amount_usd must be between 10 and 10000 with at most 2 decimals")
		return
	}
	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = "card"
	}
	userID := requestUserID(r)
	// Ensure the wallet row exists so GetWallet never 404s after a recharge.
	if _, err := model.GetOrCreateWallet(userID); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	o := &model.RechargeOrder{
		UserID:    userID,
		Amount:    amountNano,
		Status:    model.RechargePending,
		Channel:   channel,
		ExpiresAt: time.Now().Unix() + model.RechargeOrderExpirySeconds,
	}
	if err := model.CreateRechargeOrder(o); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// GetRechargeOrder returns a single order. Users can only see their own;
// admins can see any.
func GetRechargeOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	o, err := model.GetRechargeOrder(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "recharge order not found")
		return
	}
	if !canManageOrder(r, o) {
		common.ApiError(w, http.StatusForbidden, "cannot view a recharge order owned by another user")
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// ListRechargeOrders returns orders. Admins see all; users see their own.
// Stale pending/processing orders are lazily expired before returning.
func ListRechargeOrders(w http.ResponseWriter, r *http.Request) {
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
		userID = requestUserID(r)
	}
	_ = model.ExpireRechargeOrders()
	orders, err := model.ListRechargeOrders(userID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(orders))
	for i := range orders {
		data = append(data, rechargeOrderJSON(&orders[i]))
	}
	common.ApiSuccess(w, data)
}

type resolveRechargeRequest struct {
	Outcome string `json:"outcome"` // success | failed | cancelled
}

// BeginRechargePayment flips a pending order to processing (simulates
// opening the cashier). Idempotent for already-processing/terminal orders.
func BeginRechargePayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	o, err := model.GetRechargeOrder(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "recharge order not found")
		return
	}
	if !canManageOrder(r, o) {
		common.ApiError(w, http.StatusForbidden, "cannot manage a recharge order owned by another user")
		return
	}
	o, err = model.BeginRechargePayment(id)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// ResolveRechargeOrder transitions an order to its terminal state. On
// success the wallet is credited and a ledger entry is written atomically.
// Terminal states are idempotent (a repeated resolve returns the existing
// state without double-crediting).
func ResolveRechargeOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	o, err := model.GetRechargeOrder(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "recharge order not found")
		return
	}
	if !canManageOrder(r, o) {
		common.ApiError(w, http.StatusForbidden, "cannot manage a recharge order owned by another user")
		return
	}
	var req resolveRechargeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	outcome := strings.TrimSpace(req.Outcome)
	if outcome != "success" && outcome != "failed" && outcome != "cancelled" {
		common.ApiError(w, http.StatusBadRequest, "outcome must be success, failed or cancelled")
		return
	}
	o, err = model.ResolveRechargeOrder(id, outcome)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// --- helpers ---

func canManageOrder(r *http.Request, o *model.RechargeOrder) bool {
	return requestRole(r) >= common.RoleAdmin || o.UserID == requestUserID(r)
}

// rechargeOrderJSON serializes an order to the frontend JSON shape.
// Time fields are ISO strings; amount fields use both nano-USD and USD.
func rechargeOrderJSON(o *model.RechargeOrder) map[string]any {
	amountNano := o.Amount
	return map[string]any{
		"id":             o.ID,
		"order_number":   o.OrderNumber,
		"user_id":        o.UserID,
		"amount":         amountNano,
		"amount_nano":    amountNano,
		"amount_usd":     nanoToUsdString(amountNano),
		"status":         o.Status,
		"channel":        o.Channel,
		"failure_reason": o.FailureReason,
		"late_payment":   o.LatePayment,
		"created_at":     isoTime(o.CreatedAt),
		"expires_at":     isoTime(o.ExpiresAt),
		"paid_at":        isoTimeOrNil(o.PaidAt),
	}
}

// isoTime converts a unix timestamp to an ISO 8601 string. Zero → "".
func isoTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// isoTimeOrNil returns nil for zero timestamps so the JSON field is
// omitted (matching the optional `paid_at?` on the frontend type).
func isoTimeOrNil(unix int64) any {
	if unix <= 0 {
		return nil
	}
	return isoTime(unix)
}

// nanoToUsdString converts nano-USD to a decimal USD string like "100.00".
func nanoToUsdString(nano int64) string {
	whole := nano / nanoPerUSD
	frac := nano % nanoPerUSD
	if frac < 0 {
		frac = -frac
	}
	// Two decimal places for display.
	cents := frac / (nanoPerUSD / 100)
	return strconv.FormatInt(whole, 10) + "." + pad2(cents)
}

func pad2(n int64) string {
	s := strconv.FormatInt(n, 10)
	for len(s) < 2 {
		s = "0" + s
	}
	return s
}

// parseRechargeAmount validates a decimal USD string and returns nano-USD.
// Rules: $10–$10,000, at most 2 decimal places, no scientific notation.
func parseRechargeAmount(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if !rechargeAmountRe.MatchString(s) {
		return 0, false
	}
	parts := strings.SplitN(s, ".", 2)
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	// Pad fraction to 9 digits for nano conversion.
	fracPadded := frac
	for len(fracPadded) < 9 {
		fracPadded += "0"
	}
	if len(fracPadded) > 9 {
		fracPadded = fracPadded[:9]
	}
	wholeNano, err := parseInt64(whole)
	if err != nil {
		return 0, false
	}
	fracNano, err := parseInt64(fracPadded)
	if err != nil {
		return 0, false
	}
	total := wholeNano*nanoPerUSD + fracNano
	if total < 10*nanoPerUSD || total > 10000*nanoPerUSD {
		return 0, false
	}
	return total, true
}

var rechargeAmountRe = regexp.MustCompile(`^\d{1,7}(?:\.\d{1,2})?$`)

func parseInt64(s string) (int64, error) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidInt
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

var errInvalidInt = &invalidIntError{}

type invalidIntError struct{}

func (e *invalidIntError) Error() string { return "invalid integer" }
