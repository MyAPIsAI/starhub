package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"starhub/middleware"
	"starhub/model"
)

// Rewrite only the reviewed route's provider model. Public authorization,
// pricing, logs and client-facing IDs remain associated with the public model.
func resourceUpstreamCall(ch *model.Channel, public string, call upstreamCall) (upstreamCall, error) {
	if ch.RuntimeModel == "" || ch.RuntimeModel == public {
		return call, nil
	}
	if strings.HasPrefix(call.contentType, "multipart/form-data") {
		body, err := middleware.RewriteMultipartModel(call.contentType, call.body, ch.RuntimeModel)
		call.body = body
		return call, err
	}
	if ch.Type == 25 {
		// Gemini model names are validated as path components by its parser.
		old := "/models/" + public + ":"
		if !strings.Contains(call.url, old) {
			return call, fmt.Errorf("reviewed Gemini model path is unavailable")
		}
		call.url = strings.Replace(call.url, old, "/models/"+ch.RuntimeModel+":", 1)
		return call, nil
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(call.body, &body) != nil || body == nil {
		return call, fmt.Errorf("reviewed model rewrite requires a JSON object")
	}
	name, _ := json.Marshal(ch.RuntimeModel)
	body["model"] = name
	encoded, err := json.Marshal(body)
	call.body = encoded
	return call, err
}
