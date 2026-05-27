// Package jsonapi is a minimal hand-rolled JSON:API encoder/decoder tailored to
// the v1 endpoint surface. It supports single and collection resource documents,
// the errors[] envelope, and simple request decoding into a typed attrs struct.
package jsonapi

// Resource is implemented by every domain model exposed via the JSON:API
// transport. ResourceType returns the canonical type name (plural noun);
// ResourceID returns the string ID used in /<type>/<id> URLs.
type Resource interface {
	ResourceType() string
	ResourceID() string
}

// Meta holds optional metadata such as pagination.
type Meta struct {
	Page *Page `json:"page,omitempty"`
}

// Page describes pagination state.
type Page struct {
	Number       int    `json:"number,omitempty"`
	Size         int    `json:"size,omitempty"`
	TotalRecords int    `json:"total_records"`
	TotalPages   int    `json:"total_pages"`
	NextToken    string `json:"next_token,omitempty"`
}

// Links holds top-level document links.
type Links struct {
	Self string `json:"self,omitempty"`
	Next string `json:"next,omitempty"`
	Prev string `json:"prev,omitempty"`
}
