package model

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Decode before converting domain values so null cannot silently become zero or false.
func decodeResourceObject(raw []byte, target any, required ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("%w: expected object", ErrInvalidResource)
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("%w: missing %s", ErrInvalidResource, key)
		}
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%w: null %s", ErrInvalidResource, key)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (channel *ResourceChannel) UnmarshalJSON(raw []byte) error {
	type plain ResourceChannel
	var value plain
	if err := decodeResourceObject(raw, &value, "id", "name", "enabled"); err != nil {
		return err
	}
	*channel = ResourceChannel(value)
	return nil
}

func (instance *ResourceInstance) UnmarshalJSON(raw []byte) error {
	type plain ResourceInstance
	var value plain
	if err := decodeResourceObject(raw, &value, "id", "channelId", "upstreamModel", "canonicalModel", "protocols", "capabilities", "contextLength", "status", "latencyMs"); err != nil {
		return err
	}
	*instance = ResourceInstance(value)
	return nil
}

func (snapshot *ResourceSnapshot) UnmarshalJSON(raw []byte) error {
	type plain ResourceSnapshot
	var value plain
	if err := decodeResourceObject(raw, &value, "id", "sequence", "publishedAt", "channels", "instances"); err != nil {
		return err
	}
	*snapshot = ResourceSnapshot(value)
	return validateResourceSnapshot(*snapshot)
}
