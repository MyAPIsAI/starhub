package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

type publicGroupQuote struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Available    bool                  `json:"available"`
	Callable     bool                  `json:"callable"`
	Multiplier   float64               `json:"multiplier"`
	Pricing      plazaPricingJSON      `json:"pricing"`
	Protocols    []string              `json:"protocols"`
	ServiceLevel model.ServiceGroupSLA `json:"service_level"`
}

// Public catalog quotes are independent of cookies, user discounts and key scopes.
func publicCatalogRows(ctx context.Context, at time.Time) ([]plazaRow, error) {
	state, err := model.ReadPublicCatalogState(ctx, at)
	if err != nil {
		return nil, err
	}
	groups := make([]string, 0, len(state.Groups))
	for _, g := range state.Groups {
		groups = append(groups, g.GroupID)
	}
	rows := make([]plazaRow, 0, len(state.Names)+1)
	for _, name := range state.Names {
		meta := state.Metadata[name]
		standard, hasStandard := state.Standards[name]
		vendor, display := model.InferVendor(name)
		row := plazaRow{ID: name, Name: name, PublicCatalog: true, Kind: "specified", Vendor: firstNonEmpty(meta.Vendor, vendor), VendorDisplay: firstNonEmpty(meta.VendorDisplay, meta.Vendor, display), Description: meta.Description, ContextLength: meta.ContextLength, MaxOutput: meta.MaxOutput, Tags: meta.TagList(), Protocols: []string{}, AvailableGroups: []string{}, GroupPrices: []publicGroupQuote{}, Score: meta.Score, CreatedAt: meta.CreatedAt, SortWeight: meta.SortWeight, OfficialSource: meta.OfficialSourceURL, OfficialVerified: meta.OfficialVerifiedDate, OfficialConditions: meta.OfficialConditions, OfficialHistorical: meta.OfficialHistorical}
		if hasStandard {
			row.Name = firstNonEmpty(standard.Name, name)
			row.Vendor = firstNonEmpty(standard.Vendor, row.Vendor)
			row.VendorDisplay = firstNonEmpty(meta.VendorDisplay, row.Vendor)
			row.Description = firstNonEmpty(standard.Description, row.Description)
			row.ContextLength = standard.ContextLength
			row.Tags = append(row.Tags, standard.Capabilities...)
			kind := standard.Kind
			if kind == "chat" {
				kind = "text"
			}
			row.Tags = append(row.Tags, kind)
		}
		row.Tags = catalogTags(row.Tags)
		inputs := setting.ResolvePricingGroupsAt(name, groups, "", at)
		for _, g := range state.Groups {
			availability := state.Availability[name][g.GroupID]
			input := inputs[g.GroupID]
			input.GroupRatio, input.GroupGroupRatio = g.Ratio, 1
			pricing := publicCatalogPrice(input, meta, row.Tags)
			var types []int
			if availability.ChannelTypes != "" {
				if err := json.Unmarshal([]byte(availability.ChannelTypes), &types); err != nil {
					return nil, err
				}
			}
			protocols := plazaProtocols(types, row.Tags)
			if slices.Contains(types, 25) {
				protocols = append(protocols, "Gemini")
			}
			if slices.Contains(types, 1) && slices.Contains(row.Tags, "responses") {
				protocols = append(protocols, "Responses")
			}
			if hasStandard && len(standard.Protocols) > 0 {
				protocols = slices.DeleteFunc(protocols, func(p string) bool {
					if p == "Responses" {
						p = "OpenAI"
					}
					return !slices.Contains(standard.Protocols, p)
				})
			}
			quote := publicGroupQuote{ID: g.GroupID, Name: firstNonEmpty(g.Name, g.GroupID), Multiplier: g.Ratio, Available: availability.Visible(at), Callable: availability.Available, Pricing: pricing, Protocols: sortedUnique(protocols), ServiceLevel: g.ServiceLevel()}
			row.GroupPrices = append(row.GroupPrices, quote)
			row.Protocols = append(row.Protocols, protocols...)
			if quote.Callable {
				row.Callable = true
			}
			if quote.Available {
				row.IsAvailable = true
				row.AvailableGroups = append(row.AvailableGroups, g.GroupID)
				if row.PriceGroup == "" {
					row.PriceGroup = g.GroupID
					row.Pricing = pricing
				}
			}
		}
		row.Protocols = sortedUnique(row.Protocols)
		if !row.IsAvailable {
			reason := "no_public_resource"
			row.UnavailableReason = &reason
			if len(row.GroupPrices) > 0 {
				row.Pricing = row.GroupPrices[0].Pricing
			}
		}
		rows = append(rows, row)
	}
	// The named smart entry is a projection of public text candidates, never an upstream model.
	auto := plazaRow{ID: "myapis-auto", Name: "MyAPIs Auto", Vendor: "myapis", VendorDisplay: "MyAPIs", Kind: "smart", PublicCatalog: true, Tags: []string{"text"}, Protocols: []string{"OpenAI", "Anthropic"}, AvailableGroups: []string{}, GroupPrices: []publicGroupQuote{}, Pricing: plazaPricingJSON{Mode: "tiered", Currency: "USD", Explicit: true}}
	for _, g := range state.Groups {
		q := publicGroupQuote{ID: g.GroupID, Name: firstNonEmpty(g.Name, g.GroupID), Multiplier: g.Ratio, Pricing: auto.Pricing, Protocols: auto.Protocols, ServiceLevel: g.ServiceLevel()}
		for _, row := range rows {
			if !slices.Contains(row.Tags, "text") {
				continue
			}
			for _, candidate := range row.GroupPrices {
				if candidate.ID == g.GroupID {
					q.Available = q.Available || candidate.Available
					q.Callable = q.Callable || candidate.Callable
				}
			}
		}
		auto.GroupPrices = append(auto.GroupPrices, q)
		if q.Available {
			auto.IsAvailable = true
			auto.AvailableGroups = append(auto.AvailableGroups, q.ID)
			if auto.PriceGroup == "" {
				auto.PriceGroup = q.ID
			}
		}
		auto.Callable = auto.Callable || q.Callable
	}
	if !auto.IsAvailable {
		reason := "no_public_resource"
		auto.UnavailableReason = &reason
	}
	rows = append(rows, auto)
	return rows, nil
}

func catalogTags(tags []string) []string {
	for i, t := range tags {
		switch t {
		case "chat":
			tags[i] = "text"
		case "embeddings":
			tags[i] = "embedding"
		case "tts", "stt", "speech", "transcription":
			tags[i] = "audio"
		}
	}
	return sortedUnique(tags)
}

func publicCatalogPrice(input setting.PricingInputs, meta model.ModelMeta, tags []string) plazaPricingJSON {
	p := plazaPriceFromInputs(input, "", "")
	out := plazaPricingJSON{Mode: p.Mode, InputPerM: p.InputPerM, OutputPerM: p.OutputPerM, CacheReadPerM: p.CacheReadPerM, CacheWritePerM: p.CacheWritePerM, AudioInputPerM: p.AudioInputPerM, AudioOutputPerM: p.AudioOutputPerM, PerRequest: p.PerRequest, OfficialInputPerM: meta.OfficialInputPerM, OfficialOutputPerM: meta.OfficialOutputPerM, OfficialCacheReadPerM: meta.OfficialCacheReadPerM, OfficialCacheWritePerM: meta.OfficialCacheWritePerM, OfficialPerRequest: meta.OfficialPerRequest, GroupRatio: p.GroupRatio, Explicit: p.Explicit, Currency: "USD", TimePricing: p.TimePricing, Unit: "million_tokens", Expression: input.PriceExpr}
	if p.Mode == "per_request" {
		out.Unit = "request"
		if slices.Contains(tags, "video") {
			out.Unit = "second"
		}
	}
	// Image resolution/quality and expressions are conditional, not a single advertised rate.
	if input.PriceExpr != "" || (slices.Contains(tags, "image") && p.Mode == "per_request") {
		out.Mode = "tiered"
		out.InputPerM = nil
		out.OutputPerM = nil
		out.PerRequest = nil
		if slices.Contains(tags, "image") {
			out.Unit = "image"
		}
		if input.PriceExpr != "" {
			out.Expression = "(" + input.PriceExpr + ") * " + strconv.FormatFloat(p.GroupRatio, 'g', -1, 64)
		} else if p.PerRequest != nil {
			out.Expression = strconv.FormatFloat(*p.PerRequest, 'g', -1, 64) + " * size_ratio * quality_ratio * image_count"
		}
	}
	return out
}

func publicRows(w http.ResponseWriter, r *http.Request, detail bool) ([]plazaRow, bool) {
	rows, err := publicCatalogRows(r.Context(), time.Now())
	if err != nil {
		common.ApiError(w, 500, "public catalog is unavailable")
		return nil, false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !detail {
		rows = slices.DeleteFunc(rows, func(row plazaRow) bool { return !row.IsAvailable })
	}
	return rows, true
}

func PublicCatalogList(w http.ResponseWriter, r *http.Request) {
	rows, ok := publicRows(w, r, false)
	if !ok {
		return
	}
	f := parsePlazaFilters(r)
	rows = slices.DeleteFunc(rows, func(row plazaRow) bool { return !f.matches(&row) })
	sortPlazaRows(rows, parseSort(r.URL.Query().Get("sort")))
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Kind == "smart" && rows[j].Kind != "smart" })
	page, size := parsePagination(r)
	total := len(rows)
	start := min((page-1)*size, total)
	end := min(start+size, total)
	common.ApiSuccess(w, map[string]any{"items": rows[start:end], "total": total, "page": page, "page_size": size, "group": "", "cache_seconds": 60})
}

func PublicCatalogDetail(w http.ResponseWriter, r *http.Request) {
	rows, ok := publicRows(w, r, true)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "veyra-auto" {
		id = "myapis-auto"
	}
	for _, row := range rows {
		if row.ID != id {
			continue
		}
		var sla *model.ServiceGroupSLA
		for _, g := range row.GroupPrices {
			if g.ID == row.PriceGroup {
				v := g.ServiceLevel
				sla = &v
			}
		}
		status, err := model.PublicCatalogStatus(r.Context(), id, row.Tags, time.Now())
		if err != nil {
			common.ApiError(w, 500, "model status is unavailable")
			return
		}
		common.ApiSuccess(w, map[string]any{"model": row, "group": row.PriceGroup, "group_sla": sla, "status": status, "max_output": row.MaxOutput, "related": map[string]any{"same_vendor": []plazaRow{}, "same_price": []plazaRow{}}, "cache_seconds": 60})
		return
	}
	common.ApiError(w, 404, "model not found")
}

func PublicCatalogFilters(w http.ResponseWriter, r *http.Request) {
	rows, ok := publicRows(w, r, false)
	if !ok {
		return
	}
	type facet struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	vendors, tags := map[string]facet{}, map[string]facet{}
	for _, row := range rows {
		v := vendors[row.Vendor]
		v.Value = row.Vendor
		v.Label = row.VendorDisplay
		v.Count++
		vendors[row.Vendor] = v
		for _, tag := range row.Tags {
			v := tags[tag]
			v.Value = tag
			v.Label = tag
			v.Count++
			tags[tag] = v
		}
	}
	values := func(m map[string]facet) []facet {
		result := make([]facet, 0, len(m))
		for _, v := range m {
			result = append(result, v)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Value < result[j].Value })
		return result
	}
	common.ApiSuccess(w, map[string]any{"vendors": values(vendors), "tags": values(tags), "total": len(rows), "group": "", "price_histogram": []any{}})
}
