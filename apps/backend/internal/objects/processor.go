package objects

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// Pagination + share-link clamp constants. Exposed at the package
// boundary as exported symbols so the REST layer (and tests) can use
// the same numbers without duplicating literals.
const (
	// DefaultPageSize is the page size used when the caller passes 0.
	DefaultPageSize = 100
	// MaxPageSize is the upper bound the REST layer enforces; values
	// above this are silently clamped down. Mirrors MinIO's server-side
	// max-keys cap of 1000.
	MaxPageSize = 1000

	// ShareLinkMinTTL is the absolute lower bound on a share link's
	// lifetime. 30 s gives the browser enough time to actually fetch the
	// presigned URL while still preventing operators from minting
	// effectively-permanent links by accident.
	ShareLinkMinTTL = 30 * time.Second

	// DirectModeDownloadTTL is the lifetime of the presigned URL the
	// download endpoint redirects to when DownloadProxyMode == "direct".
	// Five minutes is short enough that a leaked Location header is
	// uninteresting yet long enough for slow downstream clients to
	// complete the request.
	DirectModeDownloadTTL = 5 * time.Minute
)

// s3API is the subset of *miniogo.Client (and Core) the object processor
// uses. Defining it as a local interface lets tests substitute an
// in-memory stub without standing up a fake MinIO server.
//
// ListObjectsV2 is routed through miniogo.Core in the real adapter
// because the high-level Client.ListObjects helper hides the
// continuation-token plumbing the paginated UI needs.
type s3API interface {
	ListObjectsV2(bucketName, objectPrefix, startAfter, continuationToken, delimiter string, maxkeys int) (miniogo.ListBucketV2Result, error)
	// ListObjectVersions drains the high-level WithVersions channel for a
	// single key into a slice (newest-first per S3 semantics). Implemented
	// in the wiring adapter via *miniogo.Client.ListObjects.
	ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error)
	// CopyObject performs a server-side copy (used by restore).
	CopyObject(ctx context.Context, dst miniogo.CopyDestOptions, src miniogo.CopySrcOptions) (miniogo.UploadInfo, error)
	PutObject(ctx context.Context, bucket, object string, reader io.Reader, objectSize int64, opts miniogo.PutObjectOptions) (miniogo.UploadInfo, error)
	RemoveObject(ctx context.Context, bucket, object string, opts miniogo.RemoveObjectOptions) error
	GetObject(ctx context.Context, bucket, object string, opts miniogo.GetObjectOptions) (io.ReadCloser, error)
	StatObject(ctx context.Context, bucket, object string, opts miniogo.StatObjectOptions) (miniogo.ObjectInfo, error)
	PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	// ListObjects streams a recursive (delimiter-less) listing of every
	// key under opts.Prefix. Used by the bulk-delete prefix expansion.
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
	// RemoveObjects batch-deletes the keys fed on objectsCh, signalling
	// per-key failures (and only failures) on the returned channel.
	RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
}

// ClientGetter is the concrete dependency the Processor pulls from on
// every call. The HTTP layer adapts internal/minio.Pool to this shape so
// the package never imports the live pool type; tests inject a getter
// that returns a hand-rolled stub satisfying s3API.
type ClientGetter func(ctx context.Context) (s3API, error)

// S3Client is the public face of s3API. It exists so callers outside the
// package (the HTTP wiring in cmd/harbormaster) can supply a live
// minio-go adapter to NewClientGetter without leaking the unexported
// s3API shape into the surrounding code. The live *miniogo.Client does
// not satisfy this directly because ListObjectsV2 lives on miniogo.Core;
// the adapter at the wiring site wraps both into one shape.
type S3Client interface {
	ListObjectsV2(bucketName, objectPrefix, startAfter, continuationToken, delimiter string, maxkeys int) (miniogo.ListBucketV2Result, error)
	// ListObjectVersions drains the high-level WithVersions channel for a
	// single key into a slice (newest-first per S3 semantics). Implemented
	// in the wiring adapter via *miniogo.Client.ListObjects.
	ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error)
	// CopyObject performs a server-side copy (used by restore).
	CopyObject(ctx context.Context, dst miniogo.CopyDestOptions, src miniogo.CopySrcOptions) (miniogo.UploadInfo, error)
	PutObject(ctx context.Context, bucket, object string, reader io.Reader, objectSize int64, opts miniogo.PutObjectOptions) (miniogo.UploadInfo, error)
	RemoveObject(ctx context.Context, bucket, object string, opts miniogo.RemoveObjectOptions) error
	GetObject(ctx context.Context, bucket, object string, opts miniogo.GetObjectOptions) (io.ReadCloser, error)
	StatObject(ctx context.Context, bucket, object string, opts miniogo.StatObjectOptions) (miniogo.ObjectInfo, error)
	PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	// ListObjects streams a recursive (delimiter-less) listing of every
	// key under opts.Prefix. Used by the bulk-delete prefix expansion.
	ListObjects(ctx context.Context, bucket string, opts miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo
	// RemoveObjects batch-deletes the keys fed on objectsCh, signalling
	// per-key failures (and only failures) on the returned channel.
	RemoveObjects(ctx context.Context, bucket string, objectsCh <-chan miniogo.ObjectInfo, opts miniogo.RemoveObjectsOptions) <-chan miniogo.RemoveObjectError
}

// NewClientGetter adapts a resolver that yields the public S3Client into
// a ClientGetter compatible with the unexported s3API interface used
// inside the package. This is the supported integration point for the
// HTTP layer; callers should not fabricate a ClientGetter literal
// because the underlying interface type is intentionally unexported.
func NewClientGetter(resolve func(ctx context.Context) (S3Client, error)) ClientGetter {
	return func(ctx context.Context) (s3API, error) {
		s3, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return s3, nil
	}
}

// ProcessorConfig is the immutable configuration the processor reads
// from on every relevant call. All fields are sourced from
// internal/config.Config at construction time so the processor never
// imports the config package directly.
type ProcessorConfig struct {
	// UploadMaxBytes is the per-upload ceiling enforced by wrapping the
	// request body in http.MaxBytesReader. A value <= 0 disables the
	// cap, but the config-validation layer rejects that case so the
	// processor can assume a positive value at runtime.
	UploadMaxBytes int64

	// ShareLinkMaxTTL is the operator-configured upper bound on
	// share-link lifetimes. MintShareLink clamps the caller-supplied
	// expiry to this value; the lower bound is the hard-coded
	// ShareLinkMinTTL constant.
	ShareLinkMaxTTL time.Duration

	// DownloadProxyMode is one of "proxy" | "direct". The REST layer
	// reads this to decide between streaming the object body through
	// Harbormaster (proxy) or issuing a 307 to a 5-minute presigned URL
	// (direct).
	DownloadProxyMode string
}

// Processor is the object-domain orchestrator. It depends only on the
// ClientGetter and its immutable configuration — there is no GORM DB
// here because the domain has no local persistence.
//
// Logger is used to surface best-effort operational warnings. The
// default value is a zerolog.Nop so unit tests need not configure it;
// the HTTP wire-up calls WithLogger to inject the real logger. Audit is
// the (optional) audit.Processor handle used by the REST layer to
// record actions; T3.7-3.10 leave audit wiring stubbed and T3.23 wires
// the actual Record calls.
type Processor struct {
	Clients ClientGetter
	Config  ProcessorConfig
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

// NewProcessor returns a Processor bound to pool with the given config.
// The logger defaults to zerolog.Nop; use WithLogger to attach the real
// logger. The audit handle defaults to nil; use WithAudit at the wire-up
// site once T3.23 lands.
func NewProcessor(pool ClientGetter, cfg ProcessorConfig) *Processor {
	return &Processor{Clients: pool, Config: cfg, Logger: zerolog.Nop()}
}

// WithLogger returns p with the supplied logger attached.
func (p *Processor) WithLogger(l zerolog.Logger) *Processor {
	p.Logger = l
	return p
}

// WithAudit returns p with the supplied audit processor attached. Wired
// at the HTTP construction site; T3.7-3.10 do not yet invoke Record but
// keeping the slot wired now means T3.23 can light it up without
// changing the Processor surface.
func (p *Processor) WithAudit(a *audit.Processor) *Processor {
	p.Audit = a
	return p
}

// recordAudit is a nil-safe helper. Audit writes are best-effort and must
// never surface to the operator's foreground operation.
func (p *Processor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

// List returns one page of objects under prefix inside bucket. delimiter
// is forwarded verbatim — "/" produces the directory-style view with
// CommonPrefixes populated; the empty string returns a flat key listing.
//
// pageSize is clamped to the (1, MaxPageSize] range; values <= 0 fall
// back to DefaultPageSize. pageToken is opaque: an empty string starts a
// fresh listing, any other value must come from a previous call's
// NextToken.
func (p *Processor) List(ctx context.Context, bucket, prefix, delimiter string, pageSize int, pageToken string) (ListResult, error) {
	s3, err := p.clients(ctx)
	if err != nil {
		return ListResult{}, err
	}
	pageSize = clampPageSize(pageSize)

	res, err := listObjects(ctx, s3, bucket, prefix, delimiter, pageToken, pageSize)
	if err != nil {
		return ListResult{}, mapClientError(err, "failed to list objects")
	}

	entries := make([]Entry, 0, len(res.Contents))
	for _, c := range res.Contents {
		entries = append(entries, entryFromObjectInfo(c))
	}
	prefixes := make([]Prefix, 0, len(res.CommonPrefixes))
	for _, cp := range res.CommonPrefixes {
		prefixes = append(prefixes, prefixFromCommonPrefix(cp))
	}

	next := ""
	if res.IsTruncated {
		next = res.NextContinuationToken
	}
	return ListResult{Entries: entries, Prefixes: prefixes, NextToken: next}, nil
}

// Upload streams body into bucket/key with the supplied contentType.
// The processor passes objectSize=-1 to minio-go so the client picks a
// part size and streams the body multipart; the caller is responsible
// for wrapping body in http.MaxBytesReader so the cap is enforced at
// the transport layer (the REST handler does this).
//
// On success the returned Entry mirrors the UploadInfo MinIO sent back
// (key, size, etag, last-modified) with the request-supplied
// content-type threaded through.
func (p *Processor) Upload(ctx context.Context, bucket, key string, body io.Reader, contentType, actor, sourceIP string) (Entry, error) {
	payload := map[string]any{"bucket": bucket, "key": key}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionObjectUpload,
			TargetType:     "object",
			TargetID:       bucket + "/" + key,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
	if err := ValidateObjectKey(key); err != nil {
		return Entry{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return Entry{}, failAudit(err)
	}
	info, err := putObject(ctx, s3, bucket, key, body, -1, contentType)
	if err != nil {
		return Entry{}, failAudit(mapClientError(err, "failed to upload object"))
	}
	payload["size"] = info.Size
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectUpload,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return entryFromUploadInfo(info, contentType), nil
}

// Delete removes bucket/key. Returns nil on success; a transport error
// is mapped through the standard 502 envelope. The REST layer surfaces
// success as 204 No Content.
func (p *Processor) Delete(ctx context.Context, bucket, key, actor, sourceIP string) error {
	payload := map[string]any{"bucket": bucket, "key": key}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionObjectDelete,
			TargetType:     "object",
			TargetID:       bucket + "/" + key,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
	if err := ValidateObjectKey(key); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := removeObject(ctx, s3, bucket, key); err != nil {
		return failAudit(mapClientError(err, "failed to delete object"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectDelete,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return nil
}

// Download opens a streaming reader for bucket/key together with the
// object's metadata. The metadata is fetched via a separate StatObject
// round-trip so the REST layer can set Content-Length / Content-Type
// headers before writing the body. The caller owns the returned
// ReadCloser and must Close it.
//
// versionID selects a specific version; an empty string fetches the
// current version.
//
// This is the proxy-mode path. The direct-mode path uses PresignedURL
// instead.
func (p *Processor) Download(ctx context.Context, bucket, key, versionID, actor, sourceIP string) (io.ReadCloser, Entry, error) {
	payload := map[string]any{"bucket": bucket, "key": key}
	if versionID != "" {
		payload["version_id"] = versionID
	}
	failAudit := func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         audit.ActionObjectDownloadProxy,
			TargetType:     "object",
			TargetID:       bucket + "/" + key,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
	if err := ValidateObjectKey(key); err != nil {
		return nil, Entry{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return nil, Entry{}, failAudit(err)
	}
	// Stat first so an unknown key surfaces as a typed envelope before
	// we open the (potentially large) body stream.
	info, err := statObjectVersion(ctx, s3, bucket, key, versionID)
	if err != nil {
		return nil, Entry{}, failAudit(mapClientError(err, "failed to stat object"))
	}
	rc, err := getObjectVersion(ctx, s3, bucket, key, versionID)
	if err != nil {
		return nil, Entry{}, failAudit(mapClientError(err, "failed to open object stream"))
	}
	// Success is recorded once the stream successfully opens. Per the M3
	// audit contract this represents "proxy mode, on successful
	// completion" — a stat+open success is the strongest signal the
	// processor has; a mid-stream copy failure would be visible in the
	// resource layer's access log, not the audit log.
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectDownloadProxy,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return rc, entryFromObjectInfo(info), nil
}

// PresignedURL mints a short-lived presigned GET URL for the direct-mode
// download path. The returned URL is valid for ttl; the expiry
// timestamp is computed locally so the REST layer can echo it back in
// the response without needing to parse the URL.
//
// versionID selects a specific version; an empty string presigns the
// current version.
//
// ttl is NOT clamped by ProcessorConfig.ShareLinkMaxTTL — that ceiling
// applies only to operator-minted share links, not to the internal
// 5-minute direct-mode redirect. Callers must pass a sane ttl.
func (p *Processor) PresignedURL(ctx context.Context, bucket, key, versionID string, ttl time.Duration) (string, time.Time, error) {
	if err := ValidateObjectKey(key); err != nil {
		return "", time.Time{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	var params url.Values
	if versionID != "" {
		params = url.Values{}
		params.Set("versionId", versionID)
	}
	u, err := presignedGet(ctx, s3, bucket, key, ttl, params)
	if err != nil {
		return "", time.Time{}, mapClientError(err, "failed to mint presigned URL")
	}
	return u.String(), time.Now().Add(ttl).UTC(), nil
}

// MintShareLink generates an operator-facing presigned share link with
// the requested expiry, clamped server-side to the [ShareLinkMinTTL,
// cfg.ShareLinkMaxTTL] range. The clamp happens BEFORE any audit
// payload is built so the recorded `expires_seconds` reflects the
// actual TTL minted, not the request value.
//
// Content-Disposition is set to attachment via response-content-
// disposition so the browser downloads (rather than navigates to) the
// file when the link is opened. The basename is derived from the key's
// last path segment.
func (p *Processor) MintShareLink(ctx context.Context, bucket, key string, expiresSeconds int, actor, sourceIP string) (ShareLink, error) {
	if err := ValidateObjectKey(key); err != nil {
		return ShareLink{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return ShareLink{}, err
	}
	ttl := clampShareLinkTTL(expiresSeconds, p.Config.ShareLinkMaxTTL)

	params := url.Values{}
	// response-content-disposition is the standard S3 query parameter
	// that overrides the object's stored Content-Disposition for the
	// duration of the presigned URL. We force `attachment` so a leaked
	// link doesn't render in-browser.
	params.Set("response-content-disposition", "attachment; filename=\""+path.Base(key)+"\"")

	u, err := presignedGet(ctx, s3, bucket, key, ttl, params)
	if err != nil {
		return ShareLink{}, mapClientError(err, "failed to mint share link")
	}
	// Audit policy (M3): record only success — failures here are
	// already either validation (caught above) or transport, which
	// surface in the access log. The payload carries the clamped TTL
	// (not the request value) so operators see what was actually minted.
	// The minted URL is NEVER persisted: audit.Sanitize drops any key
	// matching `url` defensively, and we don't include u.String() in
	// the payload regardless.
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionObjectShareLinkCreate,
		TargetType: "object",
		TargetID:   bucket + "/" + key,
		Outcome:    audit.OutcomeSuccess,
		PayloadSummary: map[string]any{
			"bucket":          bucket,
			"key":             key,
			"expires_seconds": int(ttl.Seconds()),
		},
	})
	return ShareLink{URL: u.String(), ExpiresAt: time.Now().Add(ttl).UTC()}, nil
}

// clampPageSize maps a caller-supplied page size to the legal range.
// Values <= 0 fall back to DefaultPageSize; values above MaxPageSize
// are clamped down.
func clampPageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return n
}

// clampShareLinkTTL maps a request `expires_seconds` value to the
// allowed [ShareLinkMinTTL, max] range. A non-positive request value
// is treated as "give me the floor" rather than "no expiry".
func clampShareLinkTTL(seconds int, max time.Duration) time.Duration {
	ttl := time.Duration(seconds) * time.Second
	if ttl < ShareLinkMinTTL {
		ttl = ShareLinkMinTTL
	}
	if max > 0 && ttl > max {
		ttl = max
	}
	return ttl
}

// clients is a tiny indirection that wraps the ClientGetter's error in
// an apierror so callers can return it directly to the HTTP layer.
func (p *Processor) clients(ctx context.Context) (s3API, error) {
	if p.Clients == nil {
		return nil, apierror.Internal("objects: client getter not configured")
	}
	s3, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return s3, nil
}

// mapClientError wraps a raw MinIO SDK error into the action-style
// apierror used by object endpoints. For now we forward the SDK message
// verbatim; a future task may translate well-known codes into typed
// envelopes (e.g. NoSuchKey -> apierror.NotFound).
func mapClientError(err error, fallback string) *apierror.Error {
	if err == nil {
		return nil
	}
	var ae *apierror.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apierror.New(http.StatusBadGateway, "minio_error", fallback+": "+err.Error())
}

// ---------------------------------------------------------------------------
// Version cursor helpers
// ---------------------------------------------------------------------------

// encodeVersionToken encodes an integer list offset as an opaque
// base64url (no-padding) string for use as a version-list page token.
func encodeVersionToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// decodeVersionToken decodes a page token produced by encodeVersionToken.
// An empty token is treated as offset 0. Any other invalid token returns
// an apierror 400.
func decodeVersionToken(tok string) (int, error) {
	if tok == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return 0, apierror.New(http.StatusBadRequest, "bad_request", "invalid page token")
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return 0, apierror.New(http.StatusBadRequest, "bad_request", "invalid page token")
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Version operations
// ---------------------------------------------------------------------------

// objectFailAudit returns a closure that records a failure audit event
// for bucket/key operations and passes the error through unchanged.
func (p *Processor) objectFailAudit(ctx context.Context, action, bucket, key string, payload map[string]any, actor, sourceIP string) func(error) error {
	return func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Actor:          actor,
			SourceIP:       sourceIP,
			Action:         action,
			TargetType:     "object",
			TargetID:       bucket + "/" + key,
			Outcome:        audit.OutcomeFailure,
			ErrorMessage:   err.Error(),
			PayloadSummary: payload,
		})
		return err
	}
}

// ListVersions returns one page of the version history for bucket/key.
// pageSize is clamped to (0, MaxPageSize]; pageToken is the opaque
// base64 offset cursor from a prior call. Sibling keys are excluded by
// listObjectVersions before the window is applied.
func (p *Processor) ListVersions(ctx context.Context, bucket, key string, pageSize int, pageToken string) (VersionListResult, error) {
	if err := ValidateObjectKey(key); err != nil {
		return VersionListResult{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
	}
	pageSize = clampPageSize(pageSize)
	offset, err := decodeVersionToken(pageToken)
	if err != nil {
		return VersionListResult{}, err
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return VersionListResult{}, err
	}
	infos, truncated, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return VersionListResult{}, mapClientError(err, "failed to list object versions")
	}

	// Map all filtered versions to domain model.
	all := make([]ObjectVersion, 0, len(infos))
	for _, info := range infos {
		all = append(all, versionFromObjectInfo(info))
	}

	// Apply page window.
	end := offset + pageSize
	if offset >= len(all) {
		return VersionListResult{Versions: []ObjectVersion{}, Truncated: truncated}, nil
	}
	if end > len(all) {
		end = len(all)
	}
	page := all[offset:end]

	nextToken := ""
	if end < len(all) {
		nextToken = encodeVersionToken(end)
	}
	return VersionListResult{Versions: page, NextToken: nextToken, Truncated: truncated}, nil
}

// RestoreVersion server-side copies srcVersionID back onto bucket/key,
// creating a new current version. Delete markers cannot be restored.
func (p *Processor) RestoreVersion(ctx context.Context, bucket, key, versionID, actor, sourceIP string) (ObjectVersion, error) {
	payload := map[string]any{"bucket": bucket, "key": key, "version_id": versionID}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectVersionRestore, bucket, key, payload, actor, sourceIP)

	if err := ValidateObjectKey(key); err != nil {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return ObjectVersion{}, failAudit(err)
	}
	infos, _, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to list object versions"))
	}
	target, found := findVersion(infos, versionID)
	if !found {
		return ObjectVersion{}, failAudit(apierror.NotFound("version"))
	}
	if target.IsDeleteMarker {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusUnprocessableEntity, "cannot_restore_delete_marker", "cannot restore a delete marker"))
	}
	uploadInfo, err := copyObjectVersion(ctx, s3, bucket, key, versionID)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to restore version"))
	}
	// Re-stat the newly-created version to get full metadata.
	info, err := statObjectVersion(ctx, s3, bucket, key, uploadInfo.VersionID)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to stat restored version"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectVersionRestore,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return versionFromObjectInfo(info), nil
}

// DeleteVersion permanently deletes a single version of bucket/key.
// confirm must be true; without it the operation is rejected to prevent
// accidental permanent deletes.
func (p *Processor) DeleteVersion(ctx context.Context, bucket, key, versionID string, confirm bool, actor, sourceIP string) error {
	payload := map[string]any{"bucket": bucket, "key": key, "version_id": versionID}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectVersionDelete, bucket, key, payload, actor, sourceIP)

	if err := ValidateObjectKey(key); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	if !confirm {
		return failAudit(apierror.New(http.StatusUnprocessableEntity, "bad_request", "permanent version delete requires confirm:true"))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := removeObjectVersion(ctx, s3, bucket, key, versionID); err != nil {
		return failAudit(mapClientError(err, "failed to delete version"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectVersionDelete,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return nil
}

// Undelete removes the latest delete marker from bucket/key, exposing
// the previously-hidden most-recent non-marker version. The key must
// currently be delete-marked (i.e. the latest version is a delete marker).
func (p *Processor) Undelete(ctx context.Context, bucket, key, actor, sourceIP string) (ObjectVersion, error) {
	payload := map[string]any{"bucket": bucket, "key": key}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectUndelete, bucket, key, payload, actor, sourceIP)

	if err := ValidateObjectKey(key); err != nil {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return ObjectVersion{}, failAudit(err)
	}
	infos, _, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to list object versions"))
	}
	latest, found := findLatest(infos)
	if !found || !latest.IsDeleteMarker {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusUnprocessableEntity, "not_delete_marked", "object is not delete-marked"))
	}
	if err := removeObjectVersion(ctx, s3, bucket, key, latest.VersionID); err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to remove delete marker"))
	}
	exposed := findNextNonMarker(infos, latest.VersionID)
	p.recordAudit(ctx, audit.Event{
		Actor:          actor,
		SourceIP:       sourceIP,
		Action:         audit.ActionObjectUndelete,
		TargetType:     "object",
		TargetID:       bucket + "/" + key,
		Outcome:        audit.OutcomeSuccess,
		PayloadSummary: payload,
	})
	return exposed, nil
}

// ---------------------------------------------------------------------------
// Version operation helpers
// ---------------------------------------------------------------------------

// findVersion searches infos for the entry with the given versionID.
func findVersion(infos []miniogo.ObjectInfo, versionID string) (miniogo.ObjectInfo, bool) {
	for _, info := range infos {
		if info.VersionID == versionID {
			return info, true
		}
	}
	return miniogo.ObjectInfo{}, false
}

// findLatest returns the entry flagged IsLatest, falling back to the
// first entry when none is explicitly flagged (WithVersions listings
// are newest-first per S3 semantics so index 0 is effectively the
// latest). Returns false only when infos is empty.
func findLatest(infos []miniogo.ObjectInfo) (miniogo.ObjectInfo, bool) {
	for _, info := range infos {
		if info.IsLatest {
			return info, true
		}
	}
	if len(infos) > 0 {
		return infos[0], true
	}
	return miniogo.ObjectInfo{}, false
}

// findNextNonMarker returns the domain ObjectVersion of the first entry
// in infos that is not the excluded versionID and is not a delete
// marker. If no such entry exists, a placeholder ObjectVersion with Key
// set to the key from infos is returned (or an empty one if infos is
// empty).
func findNextNonMarker(infos []miniogo.ObjectInfo, excludeVersionID string) ObjectVersion {
	for _, info := range infos {
		if info.VersionID == excludeVersionID {
			continue
		}
		if !info.IsDeleteMarker {
			return versionFromObjectInfo(info)
		}
	}
	// No non-marker version exists — return a placeholder carrying just the key.
	if len(infos) > 0 {
		return ObjectVersion{Key: infos[0].Key}
	}
	return ObjectVersion{}
}

// BulkDelete deletes (dryRun=false) or counts (dryRun=true) the union of
// the explicit keys and every key under each prefix.
//
// Validation runs before any MinIO call and records NO audit event on a
// pure reject: empty request -> 400 bad_request; invalid key -> 400
// object_invalid_key; empty or "/" prefix -> 400 bad_request (the
// whole-bucket-wipe guard, since an empty prefix matches every object).
//
// On dryRun the count is exact up to a 10,000 ceiling, beyond which
// ObjectCount is reported as 10000 and Truncated is set; nothing is
// deleted and no audit event is recorded. On a real delete, keys are
// issued without a version ID (delete marker on a versioned bucket,
// permanent removal otherwise), per-key failures are aggregated into
// Failures[] without aborting, a listing/transport error aborts with a
// 502 minio_error envelope, and exactly one audit event is recorded.
func (p *Processor) BulkDelete(ctx context.Context, bucket string, keys, prefixes []string, dryRun bool, actor, sourceIP string) (BulkDeleteResult, error) {
	if len(keys) == 0 && len(prefixes) == 0 {
		return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "bad_request",
			"at least one of keys or prefixes is required")
	}
	for _, k := range keys {
		if err := ValidateObjectKey(k); err != nil {
			return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
		}
	}
	for _, prefix := range prefixes {
		if prefix == "" || prefix == "/" {
			return BulkDeleteResult{}, apierror.New(http.StatusBadRequest, "bad_request", "prefix must not be empty")
		}
	}

	s3, err := p.clients(ctx)
	if err != nil {
		return BulkDeleteResult{}, err
	}

	if dryRun {
		count, truncated, cerr := countExpansion(ctx, s3, bucket, keys, prefixes)
		if cerr != nil {
			return BulkDeleteResult{}, mapClientError(cerr, "failed to count objects")
		}
		return BulkDeleteResult{ObjectCount: count, Truncated: truncated}, nil
	}

	deleted, failures, derr := deleteExpansion(ctx, s3, bucket, keys, prefixes)
	if derr != nil {
		return BulkDeleteResult{}, mapClientError(derr, "failed to bulk delete objects")
	}

	// One audit event per real delete operation. A single-prefix folder
	// delete (exactly one prefix, no explicit keys) is individually
	// traceable via bucket/prefix; everything else targets the bucket.
	targetID := bucket
	if len(prefixes) == 1 && len(keys) == 0 {
		targetID = bucket + "/" + prefixes[0]
	}
	outcome := audit.OutcomeSuccess
	if len(failures) > 0 {
		outcome = audit.OutcomeFailure
	}
	p.recordAudit(ctx, audit.Event{
		Actor:      actor,
		SourceIP:   sourceIP,
		Action:     audit.ActionObjectBulkDelete,
		TargetType: "object",
		TargetID:   targetID,
		Outcome:    outcome,
		PayloadSummary: map[string]any{
			"key_count":     len(keys),
			"prefixes":      prefixes,
			"deleted_count": deleted,
			"failure_count": len(failures),
		},
	})
	return BulkDeleteResult{DeletedCount: deleted, Failures: failures}, nil
}
