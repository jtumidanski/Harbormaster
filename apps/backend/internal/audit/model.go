// Package audit defines the domain model, persistence, and sanitization for
// audit events. Every write path calls Sanitize unconditionally so that no
// sensitive data (secrets, passwords, tokens, URLs) can ever reach the database.
package audit

import "time"

// Action constants enumerate every operation that produces an audit event.
// The writer rejects unknown actions at compile time via the typed constants.
const (
	ActionBucketCreate          = "bucket.create"
	ActionBucketDelete          = "bucket.delete"
	ActionBucketVersioningOn    = "bucket.versioning.enable"
	ActionBucketVersioningOff   = "bucket.versioning.disable"
	ActionBucketPublicUpdate    = "bucket.public_access.update"
	ActionBucketQuotaUpdate     = "bucket.quota.update"
	ActionBucketEmpty           = "bucket.empty"
	ActionObjectUpload          = "object.upload"
	ActionObjectDelete          = "object.delete"
	ActionObjectDownloadProxy   = "object.download_proxy"
	ActionObjectShareLinkCreate = "object.share_link.create"
	ActionUserCreate            = "user.create"
	ActionUserDelete            = "user.delete"
	ActionUserDisable           = "user.disable"
	ActionUserEnable            = "user.enable"
	ActionUserPoliciesUpdate    = "user.policies.update"
	ActionServiceAccountCreate  = "service_account.create"
	ActionServiceAccountRevoke  = "service_account.revoke"
	ActionLifecycleRuleCreate   = "lifecycle_rule.create"
	ActionLifecycleRuleDelete   = "lifecycle_rule.delete"
	ActionSessionLogin          = "session.login"
	ActionSessionLogout         = "session.logout"
	ActionSessionLoginFailed    = "session.login_failed"
	ActionConnectionUpdate      = "connection.update"
	ActionConnectionTest        = "connection.test"
	ActionAdminPasswordChange   = "admin.password.change"
	ActionAdminEncryptionReset  = "admin.encryption.reset"
)

// AllActions returns a slice of every defined action constant.
// Used by tests to enumerate the full action space.
func AllActions() []string {
	return []string{
		ActionBucketCreate,
		ActionBucketDelete,
		ActionBucketVersioningOn,
		ActionBucketVersioningOff,
		ActionBucketPublicUpdate,
		ActionBucketQuotaUpdate,
		ActionBucketEmpty,
		ActionObjectUpload,
		ActionObjectDelete,
		ActionObjectDownloadProxy,
		ActionObjectShareLinkCreate,
		ActionUserCreate,
		ActionUserDelete,
		ActionUserDisable,
		ActionUserEnable,
		ActionUserPoliciesUpdate,
		ActionServiceAccountCreate,
		ActionServiceAccountRevoke,
		ActionLifecycleRuleCreate,
		ActionLifecycleRuleDelete,
		ActionSessionLogin,
		ActionSessionLogout,
		ActionSessionLoginFailed,
		ActionConnectionUpdate,
		ActionConnectionTest,
		ActionAdminPasswordChange,
		ActionAdminEncryptionReset,
	}
}

// Outcome constants for audit event results.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

// Event is the immutable domain representation of a single audit record.
// PayloadSummary holds safe, non-sensitive context; it is always sanitized
// before persistence — callers must not assume that submitted payloads reach
// storage unmodified.
type Event struct {
	ID             string         // ULID (Crockford base32)
	OccurredAt     time.Time
	Actor          string
	SourceIP       string
	Action         string
	TargetType     string
	TargetID       string
	Outcome        string
	ErrorMessage   string
	PayloadSummary map[string]any
}

// Filter constrains a list query.
type Filter struct {
	Action     string
	TargetType string
	TargetID   string
	Actor      string
	Since      time.Time
	Until      time.Time
	PageSize   int
	PageOffset int
}
