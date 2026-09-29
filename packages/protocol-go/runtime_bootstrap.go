package protocol

// RuntimeVariable is one container environment entry from a revision snapshot.
// Callers must never log Value.
type RuntimeVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RuntimeBootstrap is returned by GET /agents/revisions/{revisionId}/runtime.
// Env holds snapshotted variables and the exact secret versions referenced by
// the revision. Callers must never log Env values or place them in command
// results, events, or diagnostics.
type RuntimeBootstrap struct {
	RevisionID string            `json:"revisionId"`
	Env        []RuntimeVariable `json:"env"`
}
