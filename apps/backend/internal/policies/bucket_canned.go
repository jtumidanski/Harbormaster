// Package policies defines IAM and bucket-canned policy templates and renders
// them into the JSON shapes that MinIO accepts.
package policies

import (
	"encoding/json"
	"fmt"
)

// BucketPolicyFor returns the JSON for the canned bucket policy matching the
// public-access mode. Returns ("", nil) for "private" — callers should call
// SetBucketPolicy(name, "") (or RemoveBucketPolicy) instead.
func BucketPolicyFor(bucket, mode string) (string, error) {
	switch mode {
	case "private":
		return "", nil
	case "public-read":
		return render(bucket, []string{"s3:GetObject", "s3:ListBucket"})
	case "public-read-write":
		return render(bucket, []string{"s3:GetObject", "s3:ListBucket", "s3:PutObject", "s3:DeleteObject"})
	}
	return "", fmt.Errorf("unknown public-access mode %q", mode)
}

func render(bucket string, actions []string) (string, error) {
	type stmt struct {
		Effect    string   `json:"Effect"`
		Principal any      `json:"Principal"`
		Action    []string `json:"Action"`
		Resource  []string `json:"Resource"`
	}
	type doc struct {
		Version   string `json:"Version"`
		Statement []stmt `json:"Statement"`
	}
	d := doc{
		Version: "2012-10-17",
		Statement: []stmt{{
			Effect:    "Allow",
			Principal: map[string]any{"AWS": []string{"*"}},
			Action:    actions,
			Resource:  []string{"arn:aws:s3:::" + bucket, "arn:aws:s3:::" + bucket + "/*"},
		}},
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
