package connection

// newConnectionView assembles the safe read view from an entity row and the
// decrypted credentials returned by Make. It exists so callers outside the
// package can construct a Connection from in-memory state without touching
// the package-private struct fields.
//
// All validation lives on SubmitInput (see processor.Validate); this
// builder is intentionally tiny — the immutable model has no invariants
// beyond "what the column says".
func newConnectionView(id uint, endpointURL string, tlsSkipVerify bool, creds plainCreds) Connection {
	return Connection{
		id:                 id,
		endpointURL:        endpointURL,
		tlsSkipVerify:      tlsSkipVerify,
		accessKeyMasked:    maskAccessKey(creds.AccessKey),
		secretKeyPresent:   creds.SecretKey != "",
		customCAPEMPresent: creds.CustomCAPEMText != "",
	}
}
