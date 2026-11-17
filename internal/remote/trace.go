package remote

// Tracer receives connection and command notes. Passwords must already be removed.
type Tracer interface {
	Note(level, server, kind, message, detail string)
}
