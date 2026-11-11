package dependency

// Graph declares request dependencies for one collection, keyed by request path
// in the same "Folder/Request" format workflow steps use.
type Graph struct {
	Requests map[string]Node `yaml:"requests"`
}

// Node lists the requests a request requires and the variables it produces.
type Node struct {
	Requires []string `yaml:"requires"`
	Produces []string `yaml:"produces"`
}

// SessionState answers the questions the staleness rule needs about the running session.
type SessionState interface {
	// RanSuccessfully reports whether the request has completed successfully this session.
	RanSuccessfully(requestPath string) bool
	// VariableSet reports whether the variable currently resolves to a non-empty value.
	VariableSet(name string) bool
}
