package objectstore

import "testing"

const minioShape = `{
  "lastUpdate": "2026-09-24T10:00:00Z",
  "objectsCount": 12,
  "objectsTotalSize": 4096,
  "bucketsCount": 1,
  "bucketsUsage": {
    "zot": {"size": 4096, "objectsCount": 12, "objectsPendingReplicationTotalSize": 0}
  }
}`

const rustfsShape = `{
  "last_update": "2026-09-24T10:00:00Z",
  "objects_count": 7,
  "objects_total_size": 2048,
  "buckets_count": 1,
  "buckets_usage": {
    "zot": {"size": 2048, "objects_count": 7}
  }
}`

func TestDecodeBucketsUsage_MinIOCamelCase(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(minioShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	u := got["zot"]
	if u.Size != 4096 || u.ObjectsCount != 12 {
		t.Errorf("want size 4096 / objects 12, got %+v", u)
	}
}

func TestDecodeBucketsUsage_RustFSSnakeCase(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(rustfsShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	u := got["zot"]
	if u.Size != 2048 || u.ObjectsCount != 7 {
		t.Errorf("want size 2048 / objects 7, got %+v", u)
	}
}

func TestDecodeBucketsUsage_MissingBucketIsZero(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(minioShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if u := got["nope"]; u.Size != 0 || u.ObjectsCount != 0 {
		t.Errorf("missing bucket must be zero value, got %+v", u)
	}
}

func TestDecodeBucketsUsage_BadJSON(t *testing.T) {
	if _, err := decodeBucketsUsage([]byte(`{`)); err == nil {
		t.Error("want error on malformed body")
	}
}
