package objects

import (
	"testing"

	miniogo "github.com/minio/minio-go/v7"
)

func TestVersionFromObjectInfo_DeleteMarker(t *testing.T) {
	v := versionFromObjectInfo(miniogo.ObjectInfo{
		Key: "k", VersionID: "v1", IsDeleteMarker: true, IsLatest: true,
	})
	if v.Size != nil {
		t.Errorf("delete marker Size must be nil, got %v", *v.Size)
	}
	if v.ContentType != "" {
		t.Errorf("delete marker ContentType must be empty, got %q", v.ContentType)
	}
	if !v.IsDeleteMarker || !v.IsLatest {
		t.Errorf("flags wrong: %+v", v)
	}
}

func TestVersionFromObjectInfo_Regular(t *testing.T) {
	v := versionFromObjectInfo(miniogo.ObjectInfo{
		Key: "k", VersionID: "v2", Size: 42, ContentType: "image/jpeg", IsLatest: true,
	})
	if v.Size == nil || *v.Size != 42 {
		t.Fatalf("size wrong: %+v", v)
	}
	if v.ContentType != "image/jpeg" {
		t.Errorf("content type wrong: %q", v.ContentType)
	}
}
