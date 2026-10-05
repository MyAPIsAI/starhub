package controller

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

// Model plaza API.
//
// This is a read-only browse surface over the channel graph, the metadata
// catalog and the live pricing options. It deliberately shares nothing with
// the relay path except the setting lookups: the plaza must never be able to
// affect billing, and the relay must never depend on the catalog existing.
//
// All prices are returned in USD. Currency conversion and locale formatting
// are client concerns — a server that renders "$3" cannot be cached across
// regions or currency settings.

// plazaSession resolves the browsing identity without requiring it.
//
// Anonymous visitors get the default group at list price, which is what a
// shopping visitor should see. A signed-in visitor is priced through their
// own group, including the group x group matrix, so the numbers on the page
// are the numbers they will actually be billed.
func plazaSession(r *http.Request) (userGroup string, authed bool) {
	if uid, ok := middleware.Sessions.Get(middleware.SessionToken(r)); ok {
		if u, err := model.GetUserByID(uid); err == nil && u.Status == common.StatusEnabled {
			return u.Group, true
		}
	}
	return common.DefaultGroup, false
}

// PlazaListModels handles GET /api/plaza/models.
//
// Query parameters:
//
//	q               free text over name / id / vendor / description
//	vendors         comma-separated vendor slugs
//	tags            comma-separated capability tags (AND semantics)
//	context_min     minimum context length in tokens
//	price_max       maximum input price, USD per 1M tokens
//	modes           comma-separated: per_token | per_request | tiered
//	available_only  1 to hide models the caller's group cannot reach
//	discounted_only 1 to keep only models with a published discount
//	sort            recommended | discount_desc | price_asc | context_desc | newest | name_asc
//	page, page_size pagination, page_size capped server-side
func PlazaListModels(w http.ResponseWriter, r *http.Request) {
	group, _ := plazaSession(r)
	rows, err := buildPlazaRows(group, group)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to build model catalog: "+err.Error())
		return
	}

	f := parsePlazaFilters(r)
	filtered := make([]plazaRow, 0, len(rows))
	for i := range rows {
		if f.matches(&rows[i]) {
			filtered = append(filtered, rows[i])
		}
	}

	sortPlazaRows(filtered, parseSort(r.URL.Query().Get("sort")))

	page, pageSize := parsePagination(r)
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	common.ApiSuccess(w, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"group":     group,
		"items":     filtered[start:end],
	})
}

// PlazaGetModel handles GET /api/plaza/models/{id}.
//
// The model name is taken from the path and may contain slashes and colons
// (upstream namespaces), which is why the route pattern uses a trailing
// wildcard rather than a single segment.
func PlazaGetModel(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("id"))
	if name == "" {
		common.ApiError(w, http.StatusBadRequest, "model name is required")
		return
	}

	group, _ := plazaSession(r)
	rows, err := buildPlazaRows(group, group)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to build model catalog: "+err.Error())
		return
	}

	var found *plazaRow
	for i := range rows {
		if rows[i].ID == name {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		common.ApiError(w, http.StatusNotFound, "model not found")
		return
	}

	meta, _, err := model.GetModelMeta(name)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	detail := map[string]any{"model": found, "group": group}

	// Related models: same vendor first, then price neighbours. Both lists
	// are computed from the already-filtered catalog so a visitor never gets
	// pointed at a model they cannot reach.
	relatedVendor := make([]plazaRow, 0, 6)
	relatedPrice := make([]plazaRow, 0, 6)
	target := comparableUnit(found)
	for i := range rows {
		if rows[i].ID == found.ID {
			continue
		}
		if rows[i].Vendor == found.Vendor && len(relatedVendor) < 6 {
			relatedVendor = append(relatedVendor, rows[i])
		}
		u := comparableUnit(&rows[i])
		if target > 0 && !isInf(u) && !isInf(target) {
			if diff := (u - target) / target; diff > -0.6 && diff < 0.6 && len(relatedPrice) < 6 {
				relatedPrice = append(relatedPrice, rows[i])
			}
		}
	}
	detail["related"] = map[string]any{
		"same_vendor": relatedVendor,
		"same_price":  relatedPrice,
	}

	// Tier table and display-only fields come from metadata. They are absent
	// rather than fabricated when no operator has published them.
	if meta != nil {
		detail["max_output"] = meta.MaxOutput
		detail["meta"] = map[string]any{
			"score":       meta.Score,
			"sort_weight": meta.SortWeight,
			"created_at":  meta.CreatedAt,
			"per_request": meta.PerRequestUSD,
		}
	}

	common.ApiSuccess(w, detail)
}

// PlazaFilters handles GET /api/plaza/filters.
//
// Counts are computed over the whole catalog, not over the caller's current
// selection: a server-side filter count needs the full cross-tabulation to be
// correct, and the catalog is small enough that returning global counts and
// letting the client render them is both simpler and never wrong.
func PlazaFilters(w http.ResponseWriter, r *http.Request) {
	group, _ := plazaSession(r)
	rows, err := buildPlazaRows(group, group)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type option struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}

	vendorCount := map[string]int{}
	vendorLabel := map[string]string{}
	tagCount := map[string]int{}
	contextSet := map[int64]int{}

	for i := range rows {
		vendorCount[rows[i].Vendor]++
		if _, ok := vendorLabel[rows[i].Vendor]; !ok {
			vendorLabel[rows[i].Vendor] = rows[i].VendorDisplay
		}
		for _, tag := range rows[i].Tags {
			tagCount[tag]++
		}
		if rows[i].ContextLength > 0 {
			contextSet[rows[i].ContextLength]++
		}
	}

	vendors := make([]option, 0, len(vendorCount))
	for v, c := range vendorCount {
		vendors = append(vendors, option{Value: v, Label: vendorLabel[v], Count: c})
	}
	sort.Slice(vendors, func(i, j int) bool {
		if vendors[i].Count != vendors[j].Count {
			return vendors[i].Count > vendors[j].Count
		}
		return vendors[i].Value < vendors[j].Value
	})

	tags := make([]option, 0, len(tagCount))
	for t, c := range tagCount {
		tags = append(tags, option{Value: t, Label: t, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Value < tags[j].Value
	})

	// Price histogram over comparable unit price, so the client can size a
	// log-scale slider against the real distribution instead of guessing.
	buckets := []struct {
		Label string  `json:"bucket"`
		From  float64 `json:"from"`
		To    float64 `json:"to"`
	}{{"<0.1", 0, 0.1}, {"0.1-0.5", 0.1, 0.5}, {"0.5-2", 0.5, 2}, {"2-10", 2, 10}, {">10", 10, 1e18}}
	hist := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		count := 0
		for i := range rows {
			u := comparableUnit(&rows[i])
			if isInf(u) {
				continue
			}
			if u >= b.From && u < b.To {
				count++
			}
		}
		hist = append(hist, map[string]any{"bucket": b.Label, "from": b.From, "to": b.To, "count": count})
	}

	common.ApiSuccess(w, map[string]any{
		"vendors":         vendors,
		"tags":            tags,
		"price_histogram": hist,
		"total":           len(rows),
		"group":           group,
	})
}

// PlazaRank handles GET /api/plaza/rank?type=value|hot|new.
//
// Every ranking is data-driven. A model without the required data is omitted;
// nothing is given a placeholder score to make a list look full.
func PlazaRank(w http.ResponseWriter, r *http.Request) {
	group, _ := plazaSession(r)
	rows, err := buildPlazaRows(group, group)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type rankItem struct {
		Rank        int       `json:"rank"`
		Model       *plazaRow `json:"model"`
		MetricLabel string    `json:"metric_label"`
		MetricValue string    `json:"metric_value"`
	}

	rankType := r.URL.Query().Get("type")
	items := make([]rankItem, 0, 12)

	switch rankType {
	case "hot":
		ranked := make([]plazaRow, 0, len(rows))
		for i := range rows {
			if rows[i].Calls7D != nil && *rows[i].Calls7D > 0 {
				ranked = append(ranked, rows[i])
			}
		}
		sort.SliceStable(ranked, func(i, j int) bool { return *ranked[i].Calls7D > *ranked[j].Calls7D })
		if len(ranked) > 12 {
			ranked = ranked[:12]
		}
		for i := range ranked {
			items = append(items, rankItem{
				Rank:        i + 1,
				Model:       &ranked[i],
				MetricLabel: "近 7 日调用",
				MetricValue: strconv.FormatInt(*ranked[i].Calls7D, 10),
			})
		}

	case "new":
		ranked := make([]plazaRow, 0, len(rows))
		for i := range rows {
			if rows[i].CreatedAt > 0 {
				ranked = append(ranked, rows[i])
			}
		}
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].CreatedAt > ranked[j].CreatedAt })
		if len(ranked) > 12 {
			ranked = ranked[:12]
		}
		for i := range ranked {
			items = append(items, rankItem{
				Rank:        i + 1,
				Model:       &ranked[i],
				MetricLabel: "上架时间",
				MetricValue: strconv.FormatInt(ranked[i].CreatedAt, 10),
			})
		}

	default: // value
		type scored struct {
			row   plazaRow
			index float64
		}
		ranked := make([]scored, 0, len(rows))
		for i := range rows {
			if rows[i].Score == nil {
				continue
			}
			u := comparableUnit(&rows[i])
			if isInf(u) || u <= 0 {
				continue
			}
			// Capability per dollar, on a log scale so a 10x price gap does
			// not swamp a 10-point capability gap.
			idx := *rows[i].Score / math.Max(math.Log10(u)+3, 0.5)
			ranked = append(ranked, scored{rows[i], idx})
		}
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].index > ranked[j].index })
		if len(ranked) > 12 {
			ranked = ranked[:12]
		}
		for i := range ranked {
			items = append(items, rankItem{
				Rank:        i + 1,
				Model:       &ranked[i].row,
				MetricLabel: "性价比指数",
				MetricValue: strconv.FormatFloat(ranked[i].index, 'f', 1, 64),
			})
		}
	}

	common.ApiSuccess(w, map[string]any{"type": rankType, "items": items})
}
