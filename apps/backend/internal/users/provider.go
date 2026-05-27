package users

// The users domain has NO local persistence, so the provider.go file —
// which in DB-backed packages holds the read-side queries — is empty
// here. All reads route through the Processor (which calls madmin
// directly), and there is no local index to scan.
//
// The file is preserved so the seven-file scaffold pattern stays
// consistent across packages. Future helpers (e.g. a cached username
// lookup) would live here.
