package jsonapi

import (
	"encoding/json"
	"io"
)

// Decoder reads JSON:API request documents.
type Decoder struct{}

// NewDecoder constructs a Decoder.
func NewDecoder() *Decoder { return &Decoder{} }

type singleRequest struct {
	Data struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Attributes json.RawMessage `json:"attributes"`
	} `json:"data"`
}

// Single decodes `{ "data": { "type": ..., "attributes": {...} } }` into the
// struct pointed to by out by unmarshaling attributes via encoding/json.
func (d *Decoder) Single(r io.Reader, out any) error {
	var req singleRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return err
	}
	return json.Unmarshal(req.Data.Attributes, out)
}
