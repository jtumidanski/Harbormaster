package lifecycle

// The lifecycle domain has no upstream value mappers beyond classify()
// in classifier.go. classify is the sole entry point that turns an
// mlifecycle.Rule into a domain Rule; it lives in classifier.go rather
// than here because the managed/unmanaged decision tree is the
// load-bearing logic worth isolating in its own file.
//
// If a future task adds a reverse direction (e.g. a builder that turns
// a domain Rule back into an mlifecycle.Rule for outbound mutations
// beyond the simple Expiration shape Create uses today), park those
// mappers here to keep the seven-file pattern intact.
