package policies

import (
	"encoding/json"
	"strings"
	"testing"
)

type parsedStmt struct {
	Effect    string          `json:"Effect"`
	Principal map[string][]string `json:"Principal"`
	Action    []string        `json:"Action"`
	Resource  []string        `json:"Resource"`
}

type parsedDoc struct {
	Version   string         `json:"Version"`
	Statement []parsedStmt   `json:"Statement"`
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestBucketPolicyFor_Private(t *testing.T) {
	got, err := BucketPolicyFor("my-bucket", "private")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string for private, got %q", got)
	}
}

func TestBucketPolicyFor_PublicRead(t *testing.T) {
	bucket := "my-bucket"
	raw, err := BucketPolicyFor(bucket, "public-read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty policy JSON")
	}

	var doc parsedDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(doc.Statement) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(doc.Statement))
	}
	st := doc.Statement[0]
	if st.Effect != "Allow" {
		t.Errorf("expected Effect=Allow, got %q", st.Effect)
	}
	aws, ok := st.Principal["AWS"]
	if !ok || len(aws) != 1 || aws[0] != "*" {
		t.Errorf("expected Principal.AWS=[\"*\"], got %#v", st.Principal)
	}
	if !contains(st.Action, "s3:GetObject") {
		t.Errorf("expected Action to contain s3:GetObject, got %#v", st.Action)
	}
	if !contains(st.Action, "s3:ListBucket") {
		t.Errorf("expected Action to contain s3:ListBucket, got %#v", st.Action)
	}
	if !contains(st.Resource, "arn:aws:s3:::"+bucket) {
		t.Errorf("expected Resource to contain bucket-root ARN, got %#v", st.Resource)
	}
	if !contains(st.Resource, "arn:aws:s3:::"+bucket+"/*") {
		t.Errorf("expected Resource to contain bucket-prefix ARN, got %#v", st.Resource)
	}
}

func TestBucketPolicyFor_PublicReadWrite(t *testing.T) {
	bucket := "my-bucket"
	raw, err := BucketPolicyFor(bucket, "public-read-write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc parsedDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(doc.Statement) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(doc.Statement))
	}
	st := doc.Statement[0]
	if st.Effect != "Allow" {
		t.Errorf("expected Effect=Allow, got %q", st.Effect)
	}
	aws, ok := st.Principal["AWS"]
	if !ok || len(aws) != 1 || aws[0] != "*" {
		t.Errorf("expected Principal.AWS=[\"*\"], got %#v", st.Principal)
	}
	for _, action := range []string{"s3:GetObject", "s3:ListBucket", "s3:PutObject", "s3:DeleteObject"} {
		if !contains(st.Action, action) {
			t.Errorf("expected Action to contain %s, got %#v", action, st.Action)
		}
	}
	if !contains(st.Resource, "arn:aws:s3:::"+bucket) {
		t.Errorf("expected Resource to contain bucket-root ARN, got %#v", st.Resource)
	}
	if !contains(st.Resource, "arn:aws:s3:::"+bucket+"/*") {
		t.Errorf("expected Resource to contain bucket-prefix ARN, got %#v", st.Resource)
	}
}

func TestBucketPolicyFor_UnknownMode(t *testing.T) {
	mode := "public-write-only"
	_, err := BucketPolicyFor("my-bucket", mode)
	if err == nil {
		t.Fatal("expected error for unknown mode, got nil")
	}
	if !strings.Contains(err.Error(), mode) {
		t.Errorf("expected error to contain mode %q, got %v", mode, err)
	}
}
