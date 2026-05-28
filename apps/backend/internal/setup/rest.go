package setup

// StatusResponse is the body returned by GET /api/v1/setup/status. The wire
// key is `initialized` per api-contracts.md — the SPA reads exactly that key
// to decide between the setup wizard and the login page, so it must not drift.
type StatusResponse struct {
	Initialized bool `json:"initialized"`
}

// McAliasesResponse is the body returned by GET /api/v1/setup/mc-aliases.
// UnsupportedVersion is set when the mc-config file parsed but carried a
// version other than "10" (e.g. an older mc that still ships v9 configs).
type McAliasesResponse struct {
	Aliases            []McAlias `json:"aliases"`
	UnsupportedVersion string    `json:"unsupported_version,omitempty"`
}
