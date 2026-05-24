package jsonapi

import (
	"encoding/json"
	"io"
	"strconv"
)

// Error is the JSON:API errors[] item shape used by Harbormaster.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Title   string `json:"title"`
	Detail  string `json:"detail,omitempty"`
	Pointer string `json:"-"`
}

type wireError struct {
	Status string        `json:"status"`
	Code   string        `json:"code"`
	Title  string        `json:"title"`
	Detail string        `json:"detail,omitempty"`
	Source *wireErrorSrc `json:"source,omitempty"`
}

type wireErrorSrc struct {
	Pointer string `json:"pointer,omitempty"`
}

type errorDoc struct {
	Errors []wireError `json:"errors"`
}

// WriteError writes one or more errors[] entries to w.
func WriteError(w io.Writer, errs ...Error) error {
	wires := make([]wireError, len(errs))
	for i, e := range errs {
		wires[i] = wireError{
			Status: strconv.Itoa(e.Status),
			Code:   e.Code,
			Title:  e.Title,
			Detail: e.Detail,
		}
		if e.Pointer != "" {
			wires[i].Source = &wireErrorSrc{Pointer: e.Pointer}
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(errorDoc{Errors: wires})
}
