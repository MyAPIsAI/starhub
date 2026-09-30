package controller

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

// plazaFilters is the parsed query-string view of a plaza browse request.
// Parsing is split out from the handler so the matching rules are testable
// without an HTTP round trip.
type plazaFilters struct {
	query          string
	vendors        map[string]bool
	tags           []string
	contextMin     int64
	priceMax       float64
	modes          map[string]bool
	availableOnly  bool
	discountedOnly bool
}

func parsePlazaFilters(r *http.Request) plazaFilters {
	q := r.URL.Query()
	f := plazaFilters{
		query:          strings.ToLower(strings.TrimSpace(q.Get("q"))),
		vendors:        splitSet(q.Get("vendors")),
		tags:           splitCSV(q.Get("tags")),
		contextMin:     atoi64(q.Get("context_min")),
		priceMax:       atof(q.Get("price_max")),
		modes:          splitSet(q.Get("modes")),
		availableOnly:  q.Get("available_only") == "1",
		discountedOnly: q.Get("discounted_only") == "1",
	}
	return f
}

func (f *plazaFilters) matches(row *plazaRow) bool {
	if f.query != "" {
		hay := strings.ToLower(row.Name + " " + row.ID + " " + row.Vendor + " " + row.VendorDisplay + " " + row.Description)
		if !strings.Contains(hay, f.query) {
			return false
		}
	}
	if len(f.vendors) > 0 && !f.vendors[row.Vendor] {
		return false
	}
	// Tags use AND semantics: selecting "vision" and "function_call" means
	// the model must support both. OR would make the result set grow as the
	// user narrows, which reads as a bug.
	for _, want := range f.tags {
		if !containsString(row.Tags, want) {
			return false
		}
	}
	if f.contextMin > 0 && row.ContextLength < f.contextMin {
		return false
	}
	if len(f.modes) > 0 && !f.modes[row.Pricing.Mode] {
		return false
	}
	if f.availableOnly && !row.IsAvailable {
		return false
	}
	if f.discountedOnly && discountRatio(row) <= 0 {
		return false
	}
	if f.priceMax > 0 {
		unit := comparableUnit(row)
		if math.IsInf(unit, 1) || unit > f.priceMax {
			return false
		}
	}
	return true
}

// parsePagination clamps both ends. A caller asking for a million rows gets a
// bounded response instead of exhausting memory building it.
func parsePagination(r *http.Request) (page, pageSize int) {
	q := r.URL.Query()
	page = int(atoi64(q.Get("page")))
	if page < 1 {
		page = 1
	}
	pageSize = int(atoi64(q.Get("page_size")))
	if pageSize < 1 {
		pageSize = 24
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

/* --- query parsing helpers --------------------------------------------- */

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitSet(s string) map[string]bool {
	items := splitCSV(s)
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	return set
}

func atoi64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func atof(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func isInf(v float64) bool { return math.IsInf(v, 0) || math.IsNaN(v) }
