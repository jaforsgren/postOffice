package script

import (
	"strings"

	"postOffice/internal/postman"
)

const requestPathSeparator = "/"

// RequestPath is the "folder/sub/request" path that identifies a request
// without a Postman id, in pm.info.requestId.
func RequestPath(folderPath []string, name string) string {
	return strings.Join(append(append([]string{}, folderPath...), name), requestPathSeparator)
}

// RequestScope identifies the request a script chain runs for and where its
// variables live.
type RequestScope struct {
	Collection  *postman.Collection
	Environment *postman.Environment
	FolderPath  []string
	Item        *postman.Item
	Sender      RequestSender
}

// eventScopes returns event lists outermost first: collection, each folder
// along FolderPath, then the item. A folder missing from the collection ends
// the walk; its events and anything below it are unknown.
func (s RequestScope) eventScopes() [][]postman.Event {
	var scopes [][]postman.Event
	if s.Collection != nil {
		scopes = append(scopes, s.Collection.Events)
		items := s.Collection.Items
		for _, folderName := range s.FolderPath {
			folder := findFolder(items, folderName)
			if folder == nil {
				break
			}
			scopes = append(scopes, folder.Events)
			items = folder.Items
		}
	}
	if s.Item != nil {
		scopes = append(scopes, s.Item.Events)
	}
	return scopes
}

func findFolder(items []postman.Item, name string) *postman.Item {
	for i := range items {
		if items[i].Name == name && items[i].Request == nil {
			return &items[i]
		}
	}
	return nil
}

// info backs pm.info. Postman's requestId is the item's id; items without
// one (hand-written collections) get their folder path, which
// pm.execution.setNextRequest also accepts.
func (s RequestScope) info() RequestInfo {
	if s.Item == nil {
		return RequestInfo{}
	}
	id := s.Item.ID
	if id == "" {
		id = RequestPath(s.FolderPath, s.Item.Name)
	}
	return RequestInfo{RequestName: s.Item.Name, RequestID: id}
}

// IsAddressedBy reports whether a pm.execution.setNextRequest argument names
// this request: by its name, its Postman id, or its folder path.
func (s RequestScope) IsAddressedBy(name string) bool {
	if s.Item == nil || name == "" {
		return false
	}
	return name == s.Item.Name || name == s.info().RequestID
}

// RequestRun executes one request's scripts. Both phases share a VM, so
// implicit globals a pre-request script defines (SAPI's `sapi` helpers) are
// still there in the test phase, as in Postman's per-request sandbox. Each
// phase still gets a fresh pm object.
type RequestRun struct {
	scope   RequestScope
	runtime *Runtime
}

func NewRequestRun(scope RequestScope) *RequestRun {
	return &RequestRun{scope: scope, runtime: NewRuntime()}
}

// PreRequest runs the pre-request chain for req and returns the request to
// send, with any pm.request mutations applied; req itself is untouched.
func (rr *RequestRun) PreRequest(req *postman.Request) (*postman.Request, *TestResult) {
	requestData := NewRequestData(req)
	result := rr.run(PhasePreRequest, requestData, nil)
	return requestData.ApplyTo(req), result
}

// Test runs the test chain against the sent request and its response.
func (rr *RequestRun) Test(request *RequestData, response *ResponseData) *TestResult {
	return rr.run(PhaseTest, request, response)
}

// run executes the phase's chain and writes variable changes back to the
// collection and environment.
func (rr *RequestRun) run(phase Phase, request *RequestData, response *ResponseData) *TestResult {
	scope := rr.scope
	ctx := &ExecutionContext{
		Request:     request,
		Response:    response,
		Info:        scope.info(),
		SendRequest: scope.Sender,
	}
	if scope.Collection != nil {
		ctx.CollectionVars = scope.Collection.Variables
	}
	if scope.Environment != nil {
		ctx.EnvironmentVars = scope.Environment.Values
		ctx.EnvironmentName = scope.Environment.Name
	}

	result := rr.runtime.ExecuteChain(phase, scope.eventScopes(), ctx)

	if scope.Collection != nil {
		scope.Collection.Variables = ctx.CollectionVars
	}
	if scope.Environment != nil {
		scope.Environment.Values = ctx.EnvironmentVars
	}
	return result
}
