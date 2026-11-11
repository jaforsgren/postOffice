package suggest

// Source is a document to collect field values from, such as a response body.
// Bodies that are not JSON are skipped.
type Source struct {
	Body   string
	Origin string
}

// FieldValue is a scalar value seen for a JSON field, with where it was first seen.
type FieldValue struct {
	Field  string
	Value  string
	Origin string
}

// Index holds the field values of a set of sources, most relevant first.
type Index struct {
	values []FieldValue
}
