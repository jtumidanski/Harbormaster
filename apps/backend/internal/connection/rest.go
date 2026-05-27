package connection

// GetResponse is the body returned by GET /api/v1/connection.
// SecretKeyPresent and CustomCAPEMPresent are presence flags only — the
// ciphertext columns never leave the package.
type GetResponse struct {
	EndpointURL        string `json:"endpoint_url"`
	TLSSkipVerify      bool   `json:"tls_skip_verify"`
	AccessKeyMasked    string `json:"access_key_masked"`
	SecretKeyPresent   bool   `json:"secret_key_present"`
	CustomCAPEMPresent bool   `json:"custom_ca_pem_present"`
}

// toGetResponse maps the immutable domain view onto the wire DTO.
func toGetResponse(c Connection) GetResponse {
	return GetResponse{
		EndpointURL:        c.EndpointURL(),
		TLSSkipVerify:      c.TLSSkipVerify(),
		AccessKeyMasked:    c.AccessKeyMasked(),
		SecretKeyPresent:   c.SecretKeyPresent(),
		CustomCAPEMPresent: c.CustomCAPEMPresent(),
	}
}

// UpdateRequest is the body accepted by PUT /api/v1/connection.
// Shape matches SubmitInput so the setup wizard and connection-update
// flow share a single decoder; from_mc_alias is ignored here.
type UpdateRequest struct {
	EndpointURL   string `json:"endpoint_url"`
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	TLSSkipVerify *bool  `json:"tls_skip_verify,omitempty"`
	CustomCAPEM   string `json:"custom_ca_pem,omitempty"`
}

// toSubmitInput projects the request DTO onto the package-internal
// SubmitInput consumed by the processor.
func (r UpdateRequest) toSubmitInput() SubmitInput {
	return SubmitInput{
		EndpointURL:   r.EndpointURL,
		AccessKey:     r.AccessKey,
		SecretKey:     r.SecretKey,
		TLSSkipVerify: r.TLSSkipVerify,
		CustomCAPEM:   r.CustomCAPEM,
	}
}

// TestRequest is the body accepted by POST /api/v1/connection/test.
// Identical to UpdateRequest; aliased for documentation purposes.
type TestRequest = UpdateRequest

// UpdateResponse is the body returned by PUT /api/v1/connection. The
// wizard mirrors the GET shape so the SPA can refresh state inline.
type UpdateResponse = GetResponse
