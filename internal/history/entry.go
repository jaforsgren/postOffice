package history

import (
	"postOffice/internal/postman"
	"sort"
)

// ResponseSize returns the response body size in bytes.
func (e Entry) ResponseSize() int {
	return len(e.Snapshot.Body)
}

// Request rebuilds the resolved request so it can be re-sent unchanged.
// Headers are sorted by key to keep replays deterministic.
func (e Entry) Request() *postman.Request {
	keys := make([]string, 0, len(e.Snapshot.RequestHeaders))
	for key := range e.Snapshot.RequestHeaders {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	headers := make([]postman.Header, 0, len(keys))
	for _, key := range keys {
		headers = append(headers, postman.Header{Key: key, Value: e.Snapshot.RequestHeaders[key]})
	}

	req := &postman.Request{
		Method: e.Snapshot.RequestMethod,
		Header: headers,
		URL:    postman.URL{Raw: e.Snapshot.RequestURL},
	}
	if e.Snapshot.RequestBody != "" {
		req.Body = &postman.Body{Mode: postman.BodyModeRaw, Raw: e.Snapshot.RequestBody}
	}
	return req
}
