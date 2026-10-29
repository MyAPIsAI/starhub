package dto

import (
	"reflect"
	"strings"
	"testing"
)

func TestResponsesJSONCoversEveryParsedRootField(t *testing.T) {
	for field := range reflect.TypeFor[ResponsesRequest]().Fields() {
		name := field.Tag.Get("json")
		if name == "" || name == "-" {
			continue
		}
		if err := ValidateResponsesJSON([]byte(`{"` + strings.ToUpper(name) + `":null}`)); err == nil {
			t.Fatalf("case alias for parsed field %q was accepted", name)
		}
	}
}

func TestResponsesJSONRejectsAmbiguousProtocolFields(t *testing.T) {
	for _, body := range []string{
		`{"model":"private","Model":"public"}`,
		`{"model":"public","previous_response_id":"private","Previous_response_id":""}`,
		`{"model":"public","background":true,"Background":false}`,
		`{"model":"public","store":false,"Store":true}`,
		`{"model":"public","stream":true,"Stream":false}`,
		`{"model":"public","max_output_tokens":10000,"Max_output_tokens":1}`,
		`{"model":"public","input":"hidden","Input":"visible"}`,
		`{"model":"public","conversation":{"ID":"private"}}`,
		`{"model":"public","input":[{"role":"user","Role":"assistant","content":"hidden"}]}`,
		`{"model":"public","input":[{"role":"user","content":[{"type":"input_image","Type":"input_text"}]}]}`,
		`{"model":"public","input":[{"role":"user","content":[{"type":"input_image","file_id":"private","File_id":""}]}]}`,
		`{"model":"public","tools":[{"type":"web_search","Type":"function"}]}`,
		`{"model":"public","tools":[{"type":"namespace","tools":[{"type":"web_search","Type":"function"}]}]}`,
		`{"model":"public","model":"private"}`,
		`{"model":"public","\u006dodel":"private"}`,
		`{"model":"public","metadata":{"x":1,"x":2}}`,
		`{"model":"public"} {"model":"private"}`,
		`{"model":"public","\u017ftore":false}`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := ParseResponsesRequest([]byte(body)); err == nil {
				t.Fatal("ambiguous request accepted")
			}
		})
	}
}

func TestResponsesJSONPreservesCaseSensitiveExtensionObjects(t *testing.T) {
	body := []byte(`{"model":"public","input":"hello","metadata":{"Model":"custom","model":"data"},"extension":{"number":1e1000},"tools":[{"type":"function","name":"fixture","parameters":{"type":"object","properties":{"Type":{"type":"string"},"type":{"type":"number"}}}}]}`)
	if _, err := ParseResponsesRequest(body); err != nil {
		t.Fatalf("extension data rejected: %v", err)
	}
}
