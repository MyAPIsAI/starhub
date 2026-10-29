package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/service"
)

var responseInclude = regexp.MustCompile(`^[A-Za-z0-9_.]{1,128}$`)

func responseRetrieveQuery(r *http.Request) (string, error) {
	if len(r.URL.RawQuery) > 8192 {
		return "", errors.New("response retrieval query exceeds the limit")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", errors.New("invalid response retrieval query")
	}
	for key, items := range values {
		switch key {
		case "stream":
			if len(items) != 1 || items[0] != "false" {
				return "", errors.New("streaming response retrieval is not supported yet")
			}
		case "include", "include[]":
			if len(items) > 16 {
				return "", errors.New("too many response retrieval include values")
			}
			for _, item := range items {
				if !responseInclude.MatchString(item) || strings.TrimSpace(item) != item {
					return "", errors.New("invalid response retrieval include value")
				}
			}
		case "include_obfuscation":
			if len(items) != 1 || (items[0] != "true" && items[0] != "false") {
				return "", errors.New("invalid response retrieval boolean")
			}
		default:
			return "", errors.New("unsupported response retrieval query field")
		}
	}
	return values.Encode(), nil
}

// RetrieveResponse reads only an observed response owned by this exact user and
// API key. It neither redistributes the model nor reserves/settles generation.
func RetrieveResponse(w http.ResponseWriter, r *http.Request) {
	query, err := responseRetrieveQuery(r)
	if err != nil {
		writeModeError(w, 400, "invalid_request_error", "invalid_request_body", err.Error())
		return
	}
	userID, tokenID := ctxValueInt64(r.Context(), common.CtxUserID), ctxValueInt64(r.Context(), common.CtxTokenID)
	ref, err := model.LookupResponseReference(r.Context(), userID, tokenID, r.PathValue("id"), time.Now())
	if err != nil {
		if errors.Is(err, model.ErrResponseReferenceNotFound) || errors.Is(err, model.ErrResponseReferenceAmbiguous) {
			responseNotFound(w)
		} else {
			writeModeError(w, 500, "server_error", "response_reference_unavailable", "stored response ownership could not be verified")
		}
		return
	}
	group := ctxValueString(r.Context(), common.CtxUsingGroup)
	allowedGroup := group == ref.Group
	if group == common.AutoGroup {
		groups, _ := r.Context().Value(common.CtxAutoGroups).([]string)
		for _, candidate := range groups {
			allowedGroup = allowedGroup || candidate == ref.Group
		}
	}
	limits := &model.Token{ModelLimits: ctxValueString(r.Context(), common.CtxTokenModelLimits)}
	if !allowedGroup || !limits.AllowsModel(ref.Model) {
		responseNotFound(w)
		return
	}
	definition, err := model.GetServiceGroup(ref.Group)
	if err != nil && !errors.Is(err, model.ErrServiceGroupNotFound) {
		writeModeError(w, 500, "server_error", "response_scope_unavailable", "stored response group could not be verified")
		return
	}
	definitionID := int64(0)
	if definition != nil {
		definitionID = definition.ID
	}
	if definitionID != ref.GroupDefinitionID {
		responseNotFound(w)
		return
	}
	ch, err := model.GetChannelByID(ref.ChannelID)
	if err != nil && !errors.Is(err, model.ErrChannelNotFound) {
		writeModeError(w, 500, "server_error", "response_account_unavailable", "stored response upstream could not be verified")
		return
	}
	key, ok := model.ResponseCredential(ch, ref.CredentialIdentity)
	if !ok {
		writeModeError(w, 409, "invalid_request_error", "response_account_unavailable", "the original stored response upstream account is unavailable")
		return
	}
	adaptor, err := relay.GetChatAdaptor(ch.Type)
	if err != nil {
		responseNotFound(w)
		return
	}
	endpoint, err := adaptor.ModeURL(ch.BaseURL, channel.RelayModeResponses)
	if err != nil {
		responseNotFound(w)
		return
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(ref.ResponseID)
	if query != "" {
		endpoint += "?" + query
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil || adaptor.PrepareRequest(request, key, nil) != nil {
		writeModeError(w, 502, "server_error", "response_retrieval_failed", "stored response request could not be prepared")
		return
	}
	request.Header.Set("Accept", "application/json")
	response, err := upstreamHTTPClient.Do(request)
	if err != nil {
		writeModeError(w, 502, "server_error", "response_retrieval_failed", "stored response provider could not be reached")
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		responseNotFound(w)
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeModeError(w, 502, "server_error", "response_retrieval_failed", "stored response provider returned an error")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxModeResponseBytes+1))
	var result responseAccounting
	if err != nil || len(body) > maxModeResponseBytes || json.Unmarshal(body, &result) != nil || result.ID != ref.ResponseID {
		writeModeError(w, 502, "server_error", "response_retrieval_failed", "stored response provider returned an invalid object")
		return
	}
	if !ref.Background {
		if _, terminal := responseOutcome(result.Status); !terminal {
			writeModeError(w, 502, "server_error", "response_retrieval_failed", "nonterminal stored responses are not supported yet")
			return
		}
	} else {
		task, err := model.GetTaskByID(ref.TaskID)
		if err != nil {
			writeModeError(w, 500, "server_error", "response_settlement_unavailable", "background accounting metadata is unavailable")
			return
		}
		if _, err := service.ObserveBackgroundResponse(task, body); err != nil {
			w.Header().Set("X-StarHub-Response-Settlement", "pending")
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writePassthroughBody(w, response, body)
}

func responseNotFound(w http.ResponseWriter) {
	writeModeError(w, http.StatusNotFound, "invalid_request_error", "response_not_found", "stored response is not available for this API key")
}
