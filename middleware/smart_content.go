package middleware

import "encoding/json"

// smartContentCapabilities follows structured tool-result content only. Text
// that happens to contain JSON and tool arguments are not image inputs.
func smartContentCapabilities(raw json.RawMessage) (vision, tools bool) {
	var blocks []struct {
		Type    string `json:"type"`
		Content any    `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return false, false
	}
	var visit func(string, any)
	visit = func(kind string, content any) {
		switch kind {
		case "image", "image_url":
			vision = true
		case "tool_use":
			tools = true
		case "tool_result":
			tools = true
			if children, ok := content.([]any); ok {
				for _, child := range children {
					if block, ok := child.(map[string]any); ok {
						typeName, _ := block["type"].(string)
						visit(typeName, block["content"])
					}
				}
			}
		}
	}
	for _, block := range blocks {
		visit(block.Type, block.Content)
	}
	return vision, tools
}
