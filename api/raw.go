package api

import (
	"bytes"
	"encoding/json"
)

// Each response type keeps the JSON it was decoded from in an unexported raw
// field. Its UnmarshalJSON calls decodeKeep and its MarshalJSON calls
// encodeKept, through a local type with the same fields and no methods so the
// calls do not recurse.

// decodeKeep decodes data into v and keeps a copy of data in raw.
func decodeKeep[T any](data []byte, v *T, raw *json.RawMessage) error {
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	*raw = bytes.Clone(data)
	return nil
}

// encodeKept returns raw when the value was decoded from Discogs, and encodes
// v from its fields when it was built in Go.
func encodeKept[T any](raw json.RawMessage, v T) ([]byte, error) {
	if raw != nil {
		return raw, nil
	}
	return json.Marshal(v)
}
