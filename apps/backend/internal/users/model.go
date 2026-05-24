// Package users owns the IAM-user view of MinIO: the long-term-credential
// principals that show up in `mc admin user ls`. Like the buckets domain,
// users are NOT persisted locally — MinIO is the source of truth. The User
// model in this package is assembled from madmin's ListUsers + GetUserInfo
// responses at read time.
//
// The package also hosts the service-accounts subhandler (children of a
// parent user — see serviceaccounts.go) and exposes a thin REST surface
// the SPA drives directly. Policy attachment goes through
// internal/policies' Materializer so the canonical Harbormaster naming
// scheme stays in one place.
package users

// User is the immutable read view of a single MinIO IAM user combined with
// the Harbormaster-recognised template attachments (the policies whose
// names match the harbormaster-<template>(-<param>)? naming scheme) and
// any other attached policies (e.g. operator-installed `consoleAdmin`).
//
// The secret key is intentionally absent from this model: a user's secret
// is shown to the operator exactly once at creation time and never
// retrievable thereafter — MinIO does not expose it on GetUserInfo and
// Harbormaster never persists it locally.
type User struct {
	AccessKey         string
	Status            string // "enabled" | "disabled"
	AttachedTemplates []TemplateRef
	OtherPolicies     []string
}

// TemplateRef pairs a bundled policy-template name with the params used to
// materialise it. For parameterless templates (read-only, read-write) the
// Params map is nil or empty; for backup-target it carries the bucket name.
type TemplateRef struct {
	Name   string
	Params map[string]string
}
