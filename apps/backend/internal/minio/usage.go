package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	madmin "github.com/minio/madmin-go/v4"
)

// usageRow accepts both MinIO's camelCase and RustFS's snake_case field
// names for a bucket's usage row (rustfs/rustfs#7985). Each pair is
// merged by preferring whichever is non-zero.
type usageRow struct {
	Size          uint64 `json:"size"`
	ObjectsCount  uint64 `json:"objectsCount"`
	ObjectsCount2 uint64 `json:"objects_count"`
}

type usageBody struct {
	BucketsUsage  map[string]usageRow `json:"bucketsUsage"`
	BucketsUsage2 map[string]usageRow `json:"buckets_usage"`
}

// decodeBucketsUsage parses a datausageinfo response body from either
// server flavour into madmin rows keyed by bucket.
func decodeBucketsUsage(body []byte) (map[string]madmin.BucketUsageInfo, error) {
	var b usageBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("datausageinfo: decode: %w", err)
	}
	src := b.BucketsUsage
	if len(src) == 0 {
		src = b.BucketsUsage2
	}
	out := make(map[string]madmin.BucketUsageInfo, len(src))
	for name, r := range src {
		count := r.ObjectsCount
		if count == 0 {
			count = r.ObjectsCount2
		}
		out[name] = madmin.BucketUsageInfo{Size: r.Size, ObjectsCount: count}
	}
	return out, nil
}

// BucketUsage fetches the scanner's usage census through the signed admin
// client and returns the row for bucket. A bucket the scanner has not seen
// yet is the zero value with a nil error, matching the previous
// DataUsageInfo-based adapters.
func BucketUsage(ctx context.Context, adm *madmin.AdminClient, bucket string) (madmin.BucketUsageInfo, error) {
	resp, err := adm.ExecuteMethod(ctx, http.MethodGet, madmin.RequestData{RelPath: "/v3/datausageinfo"})
	if err != nil {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: read: %w", err)
	}
	rows, err := decodeBucketsUsage(body)
	if err != nil {
		return madmin.BucketUsageInfo{}, err
	}
	return rows[bucket], nil
}
