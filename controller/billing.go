package controller

import (
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/config"
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
	owner, err := model.GetUserByID(userID)
	if err != nil {
		common.ApiError(w, 500, "account balance unavailable")
		return
	}
	loc, _ := time.LoadLocation(config.RegionTimezone())
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	entries, err := model.StatementLedger(userID, start.Unix(), start.AddDate(0, 1, 0).Unix(), "")
	if err != nil {
		common.ApiError(w, 500, "billing summary unavailable")
		return
	}
	var spent big.Int
	for _, e := range entries {
		if e.Type == "call_summary" && e.Amount < 0 {
			spent.Sub(&spent, big.NewInt(e.Amount))
		}
	}
	common.ApiSuccess(w, map[string]any{
		"month_spend_nano":               spent.String(),
		"api_balance_nano":               model.APIBalanceNano(owner).String(),
		"timezone":                       config.RegionTimezone(),
		"user_id":                        wallet.UserID,
		"balance":                        wallet.Balance, // nano-USD
		"balance_nano":                   strconv.FormatInt(wallet.Balance, 10),
		"created_at":                     wallet.CreatedAt,
		"updated_at":                     wallet.UpdatedAt,
		"api_quota_conversion_available": false,
	})
}

// --- Ledger ---

// ListLedger keeps management visibility while allowing an owner-only statement.
func ListLedger(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	if common.CanReadFinancialRecords(requestRole(r)) && r.URL.Query().Get("scope") != "owned" && r.URL.Query().Get("trace") == "" {
		userID = 0
	}
	var order *model.RechargeOrder
	for _, field := range []string{"order_id", "order_number"} {
		if values, present := r.URL.Query()[field]; present {
			if order != nil || len(values) != 1 || values[0] == "" {
				common.ApiError(w, 400, "specify a single order identifier")
				return
			}
			var ok bool
			order, ok = readRechargeOrder(w, r, values[0])
			if !ok {
				return
			}
		}
	}
	// Lazily expire stale orders so the ledger reflects reality.
	_ = model.ExpireRechargeOrders()
	var entries []model.LedgerEntry
	var err error
	if order != nil {
		entries, err = model.StatementLedgerForOrder(order, r.URL.Query().Get("trace"))
	} else {
		entries, err = model.StatementLedger(userID, 0, 0, r.URL.Query().Get("trace"))
	}
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		data = append(data, map[string]any{
			"token_id": e.TokenID, "request_count": e.RequestCount,
			"id":                 e.ID,
			"user_id":            e.UserID,
			"type":               e.Type,
			"amount":             e.Amount,
			"balance_after":      e.BalanceAfter,
			"amount_nano":        strconv.FormatInt(e.Amount, 10),
			"balance_after_nano": strconv.FormatInt(e.BalanceAfter, 10),
			"description":        e.Description,
			"order_id":           e.OrderID,
			"trace_id":           e.TraceID,
			"created_at":         e.CreatedAt,
			"funding_source":     model.PublicFundingSource(e.FundingSource),
			"api_quota_eligible": false,
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
	if !decodeBillingBody(w, r, &req) {
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
	o, ok := readRechargeOrder(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// ListRechargeOrders returns orders. Admins see all; users see their own.
// Stale pending/processing orders are lazily expired before returning.
func ListRechargeOrders(w http.ResponseWriter, r *http.Request) {
	userID := int64(0)
	if !common.CanReadFinancialRecords(requestRole(r)) || r.URL.Query().Get("scope") == "owned" {
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
	o, ok := readRechargeOrder(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	o, err := model.BeginRechargePayment(o.ID)
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
	o, ok := readRechargeOrder(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req resolveRechargeRequest
	if !decodeBillingBody(w, r, &req) {
		return
	}
	outcome := strings.TrimSpace(req.Outcome)
	if outcome != "success" && outcome != "failed" && outcome != "cancelled" {
		common.ApiError(w, http.StatusBadRequest, "outcome must be success, failed or cancelled")
		return
	}
	o, err := model.ResolveRechargeOrder(o.ID, outcome)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, rechargeOrderJSON(o))
}

// --- helpers ---

func readRechargeOrder(w http.ResponseWriter, r *http.Request, reference string) (*model.RechargeOrder, bool) {
	owner := requestUserID(r)
	if common.CanReadFinancialRecords(requestRole(r)) && r.URL.Query().Get("scope") != "owned" && r.URL.Query().Get("trace") == "" {
		owner = 0
	}
	order, err := model.FindRechargeOrder(reference, owner)
	if errors.Is(err, model.ErrRechargeOrderNotFound) {
		common.ApiError(w, http.StatusNotFound, "recharge order not found")
		return nil, false
	}
	if err != nil {
		common.ApiError(w, 500, "recharge order unavailable")
		return nil, false
	}
	return order, true
}

// rechargeOrderJSON serializes an order to the frontend JSON shape.
// Time fields are ISO strings; amount fields use both nano-USD and USD.
func rechargeOrderJSON(o *model.RechargeOrder) map[string]any {
	amountNano := o.Amount
	return map[string]any{
		"id":                 o.ID,
		"order_number":       o.OrderNumber,
		"user_id":            o.UserID,
		"amount":             amountNano,
		"amount_nano":        amountNano,
		"amount_usd":         nanoToUsdString(amountNano),
		"status":             o.Status,
		"channel":            o.Channel,
		"failure_reason":     o.FailureReason,
		"late_payment":       o.LatePayment,
		"created_at":         isoTime(o.CreatedAt),
		"expires_at":         isoTime(o.ExpiresAt),
		"paid_at":            isoTimeOrNil(o.PaidAt),
		"funding_source":     model.PublicFundingSource(o.FundingSource),
		"api_quota_eligible": false,
	}
}

func decodeBillingBody(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err == nil {
		var extra any
		if err := decoder.Decode(&extra); err == io.EOF {
			return true
		}
	}
	common.ApiError(w, http.StatusBadRequest, "invalid billing request body")
	return false
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
