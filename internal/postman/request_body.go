package postman

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const jsonNull = "null"

var ErrInvalidFormFileSource = errors.New("form file field src must be a path or a list of paths")

// FilePaths returns the files a file field uploads; none when no file is picked.
func (p FormParam) FilePaths() ([]string, error) {
	if len(p.Src) == 0 || string(p.Src) == jsonNull {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(p.Src, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(p.Src, &many); err != nil {
		return nil, fmt.Errorf("%w: field %q", ErrInvalidFormFileSource, p.Key)
	}
	return many, nil
}

// Clone returns a deep copy of r, including the fields postOffice does not model.
func (r *Request) Clone() *Request {
	if r == nil {
		return nil
	}
	copied := *r
	copied.Header = slices.Clone(r.Header)
	copied.URL.Host = slices.Clone(r.URL.Host)
	copied.URL.Path = slices.Clone(r.URL.Path)
	if r.Body != nil {
		body := *r.Body
		body.FormData = slices.Clone(r.Body.FormData)
		for i := range body.FormData {
			body.FormData[i].Src = slices.Clone(body.FormData[i].Src)
		}
		copied.Body = &body
	}
	return &copied
}
