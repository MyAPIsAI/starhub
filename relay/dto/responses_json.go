package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type responseJSONScope int

const (
	responseOpaqueJSON responseJSONScope = iota
	responseRootJSON
	responseItemJSON
	responseContentJSON
	responseConversationJSON
	responseToolJSON
)

// ValidateResponsesJSON rejects ambiguous protocol fields before routing or
// rewriting. Native forwarding and Go's case-insensitive struct decoding must
// never disagree about authorization, lifecycle or capability inputs. Extension
// objects remain case-sensitive and retain their original bytes and numbers.
func ValidateResponsesJSON(body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("Responses request must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var readValue func(responseJSONScope, int) error
	readValue = func(scope responseJSONScope, depth int) error {
		if depth > 64 {
			return errors.New("Responses JSON nesting exceeds the limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				name, ok := token.(string)
				if err != nil || !ok || seen[name] {
					return errors.New("Responses JSON contains duplicate or invalid fields")
				}
				seen[name] = true
				for _, canonical := range responseJSONFields(scope) {
					if name != canonical && strings.EqualFold(name, canonical) {
						return errors.New("Responses protocol fields must use their exact lowercase names")
					}
				}
				child := responseOpaqueJSON
				if scope == responseRootJSON {
					switch name {
					case "input":
						child = responseItemJSON
					case "conversation":
						child = responseConversationJSON
					case "tools":
						child = responseToolJSON
					}
				} else if scope == responseItemJSON && (name == "content" || name == "output" || name == "summary") {
					child = responseContentJSON
				} else if scope == responseToolJSON && name == "tools" {
					child = responseToolJSON
				}
				if err := readValue(child, depth+1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := readValue(scope, depth+1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid Responses JSON container")
		}
		_, err = decoder.Token()
		return err
	}
	if err := readValue(responseRootJSON, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("Responses request must contain exactly one JSON object")
	}
	return nil
}

func responseJSONFields(scope responseJSONScope) []string {
	switch scope {
	case responseRootJSON:
		return []string{"model", "input", "instructions", "tools", "max_output_tokens", "stream", "store", "background", "previous_response_id", "conversation", "reasoning"}
	case responseItemJSON:
		return []string{"type", "role", "content", "name", "call_id", "arguments", "input", "output", "summary", "encrypted_content"}
	case responseContentJSON:
		return []string{"type", "text", "refusal", "file_id"}
	case responseConversationJSON:
		return []string{"id"}
	case responseToolJSON:
		return []string{"type", "tools"}
	default:
		return nil
	}
}
