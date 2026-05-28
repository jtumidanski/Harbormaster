package objects

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/auth"
	"github.com/jtumidanski/Harbormaster/internal/jsonapi"
)

// actorFromRequest pulls the authenticated username and source IP off the
// session context populated by auth.RequireSession. Falls back to the raw
// remote address when no session is attached (defence in depth — these
// routes are mounted behind RequireSession, so the empty-actor path is
// only exercised in tests that drive the handler directly).
func actorFromRequest(r *http.Request) (string, string) {
	if si, ok := auth.FromContext(r.Context()); ok {
		return si.Username, si.SourceIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return "", host
}

// Routes returns a chi sub-router function that mounts the object
// endpoints under whatever parent path the caller picks. The intended
// mount point is /buckets/{bucket}/objects so the bucket name lives in
// the URL path; the bucket is read from chi.URLParam(r, "bucket") on
// every request.
//
//   - GET    /buckets/{bucket}/objects               → list
//   - POST   /buckets/{bucket}/objects               → upload (multipart)
//   - DELETE /buckets/{bucket}/objects?key=…         → delete single key
//   - GET    /buckets/{bucket}/objects/download?key= → download (proxy/direct)
//   - POST   /buckets/{bucket}/objects/share-links   → mint share link
//
// All collection / single endpoints render application/vnd.api+json
// using the jsonapi codec; error envelopes also follow the JSON:API
// errors[] shape via apierror.StyleJSONAPI.
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p, enc: jsonapi.NewEncoder()}
	return func(r chi.Router) {
		r.Get("/buckets/{bucket}/objects", h.list)
		r.Post("/buckets/{bucket}/objects", h.upload)
		r.Delete("/buckets/{bucket}/objects", h.delete)
		r.Get("/buckets/{bucket}/objects/download", h.download)
		r.Post("/buckets/{bucket}/objects/share-links", h.shareLink)
	}
}

// handler bundles the processor with the JSON:API encoder so each HTTP
// method doesn't have to reconstruct it per-request.
type handler struct {
	p   *Processor
	enc *jsonapi.Encoder
}

// list returns one page of objects under the requested prefix as a
// JSON:API collection document. The data[] array mixes object_entries
// and object_prefixes resources; meta.page.next_token carries the
// opaque continuation token (empty when the listing is exhausted) and
// links.next echoes the URL the SPA can navigate to for the next page.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	q := r.URL.Query()
	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	pageSize, _ := strconv.Atoi(q.Get("page[size]"))
	pageToken := q.Get("page[token]")

	res, err := h.p.List(r.Context(), bucket, prefix, delimiter, pageSize, pageToken)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}

	resources := make([]jsonapi.Resource, 0, len(res.Entries)+len(res.Prefixes))
	for _, e := range res.Entries {
		resources = append(resources, entryResource{Entry: e})
	}
	for _, p := range res.Prefixes {
		resources = append(resources, prefixResource{Prefix: p})
	}

	meta := &jsonapi.Meta{Page: &jsonapi.Page{
		Size:         clampPageSize(pageSize),
		TotalRecords: len(resources),
		NextToken:    res.NextToken,
	}}
	var links *jsonapi.Links
	if res.NextToken != "" {
		links = &jsonapi.Links{Next: buildNextURL(r, res.NextToken)}
	}

	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, meta, links)
}

// upload accepts a multipart/form-data POST with the canonical fields:
//
//   - key          (text)    — the object key under which to store the upload
//   - file         (file)    — the upload body
//   - content_type (text)    — optional explicit MIME type
//
// The request body is wrapped in http.MaxBytesReader so the configured
// HARBORMASTER_UPLOAD_MAX_BYTES ceiling is enforced at the transport
// layer. A body that exceeds the cap surfaces as 413 upload_too_large
// with details.limit_bytes set to the ceiling; the 413 is rendered
// action-style (per api-contracts.md) so the SPA can surface the configured
// limit, even though the rest of this endpoint surface uses JSON:API errors.
func (h *handler) upload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	cap := h.p.Config.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, cap)

	// ParseMultipartForm with maxMemory=32MiB matches the stdlib default;
	// large file parts spill to disk under r.Body, which MaxBytesReader
	// still polices.
	const maxMemory = 32 << 20
	//nolint:gosec // G120: r.Body is wrapped in http.MaxBytesReader (cap=UploadMaxBytes) above, so form parsing is bounded; oversize surfaces as 413 upload_too_large.
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		if isMaxBytesError(err) {
			// Action-style so details.limit_bytes reaches the SPA; the
			// JSON:API errors[] envelope drops the Details map, which left
			// the upload dialog always showing the hardcoded default cap.
			apierror.Write(w, apierror.StyleAction, apierror.New(
				http.StatusRequestEntityTooLarge,
				"upload_too_large",
				fmt.Sprintf("upload exceeds the configured maximum of %d bytes", cap),
			).WithDetails(map[string]any{"limit_bytes": cap}))
			return
		}
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(
			http.StatusBadRequest, "bad_request",
			"invalid multipart body: "+err.Error()))
		return
	}

	key := r.FormValue("key")
	if key == "" {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(
			http.StatusBadRequest, "bad_request",
			"multipart field 'key' is required"))
		return
	}
	contentType := r.FormValue("content_type")

	file, fhdr, err := r.FormFile("file")
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(
			http.StatusBadRequest, "bad_request",
			"multipart field 'file' is required: "+err.Error()))
		return
	}
	defer file.Close()

	if contentType == "" && fhdr != nil {
		contentType = fhdr.Header.Get("Content-Type")
	}

	// Wrap the file reader in MaxBytesReader as a defence-in-depth
	// guard: even though r.Body is already wrapped, a large multipart
	// envelope may overshoot the cap on a single file part. The cap is
	// still the same ceiling.
	limited := http.MaxBytesReader(w, io.NopCloser(file), cap)
	defer limited.Close()

	actor, ip := actorFromRequest(r)
	entry, err := h.p.Upload(r.Context(), bucket, key, limited, contentType, actor, ip)
	if err != nil {
		if isMaxBytesError(err) {
			// Action-style so details.limit_bytes reaches the SPA; the
			// JSON:API errors[] envelope drops the Details map, which left
			// the upload dialog always showing the hardcoded default cap.
			apierror.Write(w, apierror.StyleAction, apierror.New(
				http.StatusRequestEntityTooLarge,
				"upload_too_large",
				fmt.Sprintf("upload exceeds the configured maximum of %d bytes", cap),
			).WithDetails(map[string]any{"limit_bytes": cap}))
			return
		}
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusCreated)
	_ = h.enc.Single(w, entryResource{Entry: entry}, nil)
}

// delete removes a single object. The key is carried in the query
// string (not the URL path) because object keys can contain slashes and
// URL-encoding them produces brittle round-trips through chi's path
// parser; the query-string approach mirrors how the SPA already builds
// the URL.
//
// Success is 204 No Content with no body. Errors render via the action
// envelope so the SPA can read `error.code` directly.
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	key := r.URL.Query().Get("key")
	actor, ip := actorFromRequest(r)
	if err := h.p.Delete(r.Context(), bucket, key, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// download serves either a proxied byte stream or a 307 redirect to a
// short-lived presigned URL, depending on the operator-configured
// DownloadProxyMode. Proxy mode streams through Harbormaster (the
// network egress goes through us); direct mode hands the browser a
// presigned URL it loads directly from MinIO.
func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	key := r.URL.Query().Get("key")

	switch h.p.Config.DownloadProxyMode {
	case "direct":
		urlStr, _, err := h.p.PresignedURL(r.Context(), bucket, key, DirectModeDownloadTTL)
		if err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		w.Header().Set("Location", urlStr)
		// Tell intermediaries not to cache the redirect; the underlying
		// presigned URL is short-lived and per-request.
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusTemporaryRedirect)
	default:
		// "proxy" (and any unrecognised value as a safe default).
		actor, ip := actorFromRequest(r)
		rc, entry, err := h.p.Download(r.Context(), bucket, key, actor, ip)
		if err != nil {
			apierror.Write(w, apierror.StyleAction, err)
			return
		}
		defer rc.Close()

		if entry.ContentType != "" {
			w.Header().Set("Content-Type", entry.ContentType)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if entry.Size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
		}
		w.Header().Set("Content-Disposition",
			"attachment; filename=\""+path.Base(key)+"\"")
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}
}

// shareLink mints an operator-facing share link. The body is a plain
// JSON document; the response is a JSON:API single-resource document of
// type object_share_links.
func (h *handler) shareLink(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var body ShareLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(
			http.StatusBadRequest, "bad_request", "invalid JSON body"))
		return
	}

	actor, ip := actorFromRequest(r)
	sl, err := h.p.MintShareLink(r.Context(), bucket, body.Key, body.ExpiresSeconds, actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusCreated)
	_ = h.enc.Single(w, shareLinkResource{ShareLink: sl}, nil)
}

// isMaxBytesError returns true when err originated from
// http.MaxBytesReader's cap. The reader returns the typed
// *http.MaxBytesError wrapped inside ParseMultipartForm's error chain;
// errors.As walks the chain so we don't have to know the exact wrapper.
func isMaxBytesError(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// buildNextURL synthesises the absolute URL the client should hit to
// fetch the next page. We rewrite only the page[token] query parameter
// so prefix / delimiter / page[size] survive unchanged.
func buildNextURL(r *http.Request, nextToken string) string {
	cp := *r.URL
	q := cp.Query()
	q.Set("page[token]", nextToken)
	cp.RawQuery = q.Encode()
	// r.URL is path+query only; the Host comes from r.Host. Fall back
	// to the relative form if Host is empty (httptest et al.).
	if r.Host != "" {
		u := &url.URL{
			Scheme:   schemeFromRequest(r),
			Host:     r.Host,
			Path:     cp.Path,
			RawQuery: cp.RawQuery,
		}
		return u.String()
	}
	return cp.String()
}

// schemeFromRequest infers https vs http from the request. r.TLS is the
// authoritative signal locally; behind a reverse proxy the deployment
// is responsible for stripping X-Forwarded-* before us (the trusted-
// proxies config in internal/server handles that), so we don't honour
// the header here.
func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
