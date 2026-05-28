package buckets

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// listParams is the parsed pagination + sort for GET /buckets.
type listParams struct {
	number int
	size   int
	sort   string
}

// parseListParams reads the JSON:API pagination + sort query params, applying
// the documented defaults (page 1, size 50, sort by name) and clamping size to
// maxPageSize. Malformed values fall back to defaults rather than erroring.
func parseListParams(q url.Values) listParams {
	p := listParams{number: 1, size: defaultPageSize, sort: "name"}
	if v, err := strconv.Atoi(q.Get("page[number]")); err == nil && v > 0 {
		p.number = v
	}
	if v, err := strconv.Atoi(q.Get("page[size]")); err == nil && v > 0 {
		p.size = v
		if p.size > maxPageSize {
			p.size = maxPageSize
		}
	}
	if s := q.Get("sort"); s != "" {
		p.sort = s
	}
	return p
}

// sortBuckets orders bs in place per the sort param. A leading "-" sorts
// descending. Recognised fields: name (default), created/created_at,
// size/estimated_bytes, objects/object_count. Unknown fields fall back to
// name ascending so a bad sort param degrades gracefully rather than 500ing.
func sortBuckets(bs []Bucket, sortParam string) {
	desc := strings.HasPrefix(sortParam, "-")
	field := strings.TrimPrefix(sortParam, "-")
	var less func(a, b Bucket) bool
	switch field {
	case "created", "created_at":
		less = func(a, b Bucket) bool { return a.CreatedAt.Before(b.CreatedAt) }
	case "size", "estimated_bytes":
		less = func(a, b Bucket) bool { return a.EstimatedBytes < b.EstimatedBytes }
	case "objects", "object_count":
		less = func(a, b Bucket) bool { return a.ObjectCount < b.ObjectCount }
	default:
		less = func(a, b Bucket) bool { return a.Name < b.Name }
	}
	sort.SliceStable(bs, func(i, j int) bool {
		if desc {
			return less(bs[j], bs[i])
		}
		return less(bs[i], bs[j])
	})
}

// pageOf returns the slice for the requested 1-indexed page and the total
// page count (always >= 1). An out-of-range page yields an empty slice.
func pageOf(bs []Bucket, number, size int) ([]Bucket, int) {
	total := len(bs)
	totalPages := (total + size - 1) / size
	if totalPages < 1 {
		totalPages = 1
	}
	start := (number - 1) * size
	if start >= total {
		return []Bucket{}, totalPages
	}
	end := start + size
	if end > total {
		end = total
	}
	return bs[start:end], totalPages
}
