package jsonapi

import (
	"encoding/json"
	"io"
)

// Encoder writes JSON:API documents.
type Encoder struct{}

// NewEncoder constructs an Encoder.
func NewEncoder() *Encoder { return &Encoder{} }

type resourceDoc struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes any    `json:"attributes"`
}

type singleDoc struct {
	Data  resourceDoc `json:"data"`
	Meta  *Meta       `json:"meta,omitempty"`
	Links *Links      `json:"links,omitempty"`
}

type collectionDoc struct {
	Data  []resourceDoc `json:"data"`
	Meta  *Meta         `json:"meta,omitempty"`
	Links *Links        `json:"links,omitempty"`
}

// Single writes a single-resource JSON:API document.
// The `r` value's serialized JSON form (via encoding/json) becomes `attributes`,
// minus the resource ID (which is excluded by convention by tagging it `json:"-"`
// on the model struct).
func (e *Encoder) Single(w io.Writer, r Resource, links *Links) error {
	doc := singleDoc{Data: resourceDoc{Type: r.ResourceType(), ID: r.ResourceID(), Attributes: r}, Links: links}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

// Collection writes a collection JSON:API document.
func (e *Encoder) Collection(w io.Writer, rs []Resource, meta *Meta, links *Links) error {
	docs := make([]resourceDoc, len(rs))
	for i, r := range rs {
		docs[i] = resourceDoc{Type: r.ResourceType(), ID: r.ResourceID(), Attributes: r}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(collectionDoc{Data: docs, Meta: meta, Links: links})
}
