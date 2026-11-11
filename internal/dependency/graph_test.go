package dependency

import (
	"errors"
	"reflect"
	"testing"
)

const (
	loginPath   = "Auth/Login"
	profilePath = "Users/Profile"
	getUserPath = "Users/Get User"
	tokenVar    = "token"
	userIDVar   = "userId"
)

type fakeSession struct {
	succeeded map[string]bool
	variables map[string]bool
}

func (s fakeSession) RanSuccessfully(requestPath string) bool { return s.succeeded[requestPath] }
func (s fakeSession) VariableSet(name string) bool            { return s.variables[name] }

func newFakeSession() fakeSession {
	return fakeSession{succeeded: map[string]bool{}, variables: map[string]bool{}}
}

// chainGraph: Get User → Profile → Login, Get User → Login.
func chainGraph() *Graph {
	return &Graph{Requests: map[string]Node{
		loginPath:   {Produces: []string{tokenVar}},
		profilePath: {Requires: []string{loginPath}, Produces: []string{userIDVar}},
		getUserPath: {Requires: []string{profilePath, loginPath}},
	}}
}

func allExist(string) bool { return true }

func TestIsStale(t *testing.T) {
	g := chainGraph()
	tests := []struct {
		name      string
		succeeded bool
		tokenSet  bool
		want      bool
	}{
		{"never ran", false, true, true},
		{"ran but produced variable empty", true, false, true},
		{"ran and produced variable set", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newFakeSession()
			s.succeeded[loginPath] = tt.succeeded
			s.variables[tokenVar] = tt.tokenSet
			if got := g.IsStale(loginPath, s); got != tt.want {
				t.Errorf("IsStale = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsStale_UndeclaredRequestDependsOnlyOnSuccess(t *testing.T) {
	g := NewGraph()
	s := newFakeSession()
	if !g.IsStale(loginPath, s) {
		t.Error("expected request that never ran to be stale")
	}
	s.succeeded[loginPath] = true
	if g.IsStale(loginPath, s) {
		t.Error("expected request without produced variables to be fresh after success")
	}
}

func TestUnsetProducedVariables(t *testing.T) {
	g := &Graph{Requests: map[string]Node{loginPath: {Produces: []string{tokenVar, userIDVar}}}}
	s := newFakeSession()
	s.variables[userIDVar] = true
	if got := g.UnsetProducedVariables(loginPath, s); !reflect.DeepEqual(got, []string{tokenVar}) {
		t.Errorf("UnsetProducedVariables = %v, want [%s]", got, tokenVar)
	}
}

func TestStalePrerequisites_AllStaleInTopologicalOrder(t *testing.T) {
	got, err := chainGraph().StalePrerequisites(getUserPath, newFakeSession())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{loginPath, profilePath}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("StalePrerequisites = %v, want %v", got, want)
	}
}

func TestStalePrerequisites_SkipsFreshPrerequisites(t *testing.T) {
	s := newFakeSession()
	s.succeeded[loginPath] = true
	s.variables[tokenVar] = true

	got, err := chainGraph().StalePrerequisites(getUserPath, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{profilePath}; !reflect.DeepEqual(got, want) {
		t.Errorf("StalePrerequisites = %v, want %v", got, want)
	}
}

func TestStalePrerequisites_DoesNotDescendIntoFreshPrerequisite(t *testing.T) {
	s := newFakeSession()
	s.succeeded[profilePath] = true
	s.variables[userIDVar] = true
	g := &Graph{Requests: map[string]Node{
		profilePath: {Requires: []string{loginPath}, Produces: []string{userIDVar}},
		getUserPath: {Requires: []string{profilePath}},
	}}

	got, err := g.StalePrerequisites(getUserPath, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no stale prerequisites behind a fresh one, got %v", got)
	}
}

func TestStalePrerequisites_NoDependencies(t *testing.T) {
	got, err := NewGraph().StalePrerequisites(getUserPath, newFakeSession())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no prerequisites, got %v", got)
	}
}

func TestStalePrerequisites_Cycle(t *testing.T) {
	g := &Graph{Requests: map[string]Node{
		loginPath:   {Requires: []string{getUserPath}},
		getUserPath: {Requires: []string{loginPath}},
	}}

	_, err := g.StalePrerequisites(getUserPath, newFakeSession())
	var cycleErr *CycleError
	if !errors.As(err, &cycleErr) {
		t.Fatalf("expected CycleError, got %v", err)
	}
	want := []string{getUserPath, loginPath, getUserPath}
	if !reflect.DeepEqual(cycleErr.Path, want) {
		t.Errorf("cycle path = %v, want %v", cycleErr.Path, want)
	}
	if cycleErr.Error() != "dependency cycle: Users/Get User → Auth/Login → Users/Get User" {
		t.Errorf("unexpected message: %s", cycleErr.Error())
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := chainGraph().Validate(allExist); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_SelfDependencyIsCycle(t *testing.T) {
	g := &Graph{Requests: map[string]Node{loginPath: {Requires: []string{loginPath}}}}
	var cycleErr *CycleError
	if err := g.Validate(allExist); !errors.As(err, &cycleErr) {
		t.Fatalf("expected CycleError, got %v", err)
	}
	if want := []string{loginPath, loginPath}; !reflect.DeepEqual(cycleErr.Path, want) {
		t.Errorf("cycle path = %v, want %v", cycleErr.Path, want)
	}
}

func TestValidate_CycleOutsideEntryPoint(t *testing.T) {
	g := &Graph{Requests: map[string]Node{
		"A": {Requires: []string{"B"}},
		"B": {Requires: []string{"C"}},
		"C": {Requires: []string{"B"}},
	}}
	var cycleErr *CycleError
	if err := g.Validate(allExist); !errors.As(err, &cycleErr) {
		t.Fatalf("expected CycleError, got %v", err)
	}
	if want := []string{"B", "C", "B"}; !reflect.DeepEqual(cycleErr.Path, want) {
		t.Errorf("cycle path = %v, want %v", cycleErr.Path, want)
	}
}

func TestValidate_MissingReferences(t *testing.T) {
	g := &Graph{Requests: map[string]Node{
		getUserPath: {Requires: []string{"Auth/Logn"}},
		"Typo/Req":  {},
	}}
	exists := func(path string) bool { return path == getUserPath }

	err := g.Validate(exists)

	var missing *MissingPrerequisiteError
	if !errors.As(err, &missing) {
		t.Fatalf("expected MissingPrerequisiteError, got %v", err)
	}
	if missing.Request != "Auth/Logn" || missing.RequiredBy != getUserPath {
		t.Errorf("unexpected missing prerequisite: %+v", missing)
	}
	var unknown *UnknownRequestError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected UnknownRequestError, got %v", err)
	}
	if unknown.Request != "Typo/Req" {
		t.Errorf("unexpected unknown request: %+v", unknown)
	}
}
