package dependency

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

const (
	loginPath   = "Auth/Login"
	profilePath = "Users/Profile"
	getUserPath = "Users/Get User"
	tokenVar    = "token"
	userIDVar   = "userId"
)

type fakeSession struct {
	succeeded map[string]time.Time
	variables map[string]bool
	changed   map[string][]string
	now       time.Time
}

func (s fakeSession) LastSuccess(requestPath string) (time.Time, bool) {
	at, ok := s.succeeded[requestPath]
	return at, ok
}
func (s fakeSession) VariableSet(name string) bool              { return s.variables[name] }
func (s fakeSession) ChangedInputs(requestPath string) []string { return s.changed[requestPath] }
func (s fakeSession) Now() time.Time                            { return s.now }
func (s fakeSession) succeed(requestPath string, ago time.Duration) {
	s.succeeded[requestPath] = s.now.Add(-ago)
}

func newFakeSession() fakeSession {
	return fakeSession{
		succeeded: map[string]time.Time{},
		variables: map[string]bool{},
		changed:   map[string][]string{},
		now:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func requires(paths ...string) []Requirement {
	requirements := make([]Requirement, len(paths))
	for i, path := range paths {
		requirements[i] = Requirement{Request: path}
	}
	return requirements
}

// chainGraph: Get User → Profile → Login, Get User → Login.
func chainGraph() *Graph {
	return &Graph{Requests: map[string]Node{
		loginPath:   {Produces: []string{tokenVar}},
		profilePath: {Requires: requires(loginPath), Produces: []string{userIDVar}},
		getUserPath: {Requires: requires(profilePath, loginPath)},
	}}
}

func allExist(string) bool { return true }

func TestStaleness(t *testing.T) {
	const maxAge = 15 * time.Minute
	tests := []struct {
		name        string
		requirement Requirement
		arrange     func(s fakeSession)
		want        Staleness
	}{
		{
			name:        "never ran",
			requirement: Requirement{Request: loginPath},
			arrange:     func(s fakeSession) { s.variables[tokenVar] = true },
			want:        Staleness{Reason: NeverRan},
		},
		{
			name:        "ran but produced variable empty",
			requirement: Requirement{Request: loginPath},
			arrange:     func(s fakeSession) { s.succeed(loginPath, time.Minute) },
			want:        Staleness{Reason: ProducedUnset, Variables: []string{tokenVar}},
		},
		{
			name:        "ran and produced variable set",
			requirement: Requirement{Request: loginPath},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, time.Hour)
				s.variables[tokenVar] = true
			},
			want: Staleness{},
		},
		{
			name:        "older than max_age",
			requirement: Requirement{Request: loginPath, MaxAge: maxAge},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, maxAge+time.Second)
				s.variables[tokenVar] = true
			},
			want: Staleness{Reason: Expired, Age: maxAge + time.Second, MaxAge: maxAge},
		},
		{
			name:        "within max_age",
			requirement: Requirement{Request: loginPath, MaxAge: maxAge},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, maxAge)
				s.variables[tokenVar] = true
			},
			want: Staleness{},
		},
		{
			name:        "inputs changed since the last success",
			requirement: Requirement{Request: loginPath},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, time.Minute)
				s.variables[tokenVar] = true
				s.changed[loginPath] = []string{"username"}
			},
			want: Staleness{Reason: InputsChanged, Variables: []string{"username"}},
		},
		{
			name:        "changed produced variable is not an input change",
			requirement: Requirement{Request: loginPath},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, time.Minute)
				s.variables[tokenVar] = true
				s.changed[loginPath] = []string{tokenVar}
			},
			want: Staleness{},
		},
		{
			name:        "when_missing ignores that it never ran",
			requirement: Requirement{Request: loginPath, WhenMissing: []string{tokenVar}},
			arrange:     func(s fakeSession) { s.variables[tokenVar] = true },
			want:        Staleness{},
		},
		{
			name:        "when_missing variable unset",
			requirement: Requirement{Request: loginPath, WhenMissing: []string{tokenVar, userIDVar}},
			arrange: func(s fakeSession) {
				s.succeed(loginPath, time.Minute)
				s.variables[tokenVar] = true
			},
			want: Staleness{Reason: VariablesMissing, Variables: []string{userIDVar}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newFakeSession()
			tt.arrange(s)
			if got := chainGraph().Staleness(tt.requirement, s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Staleness = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStaleness_UndeclaredRequestDependsOnlyOnSuccess(t *testing.T) {
	g := NewGraph()
	s := newFakeSession()
	requirement := Requirement{Request: loginPath}
	if !g.Staleness(requirement, s).IsStale() {
		t.Error("expected request that never ran to be stale")
	}
	s.succeed(loginPath, time.Minute)
	if g.Staleness(requirement, s).IsStale() {
		t.Error("expected request without produced variables to be fresh after success")
	}
}

func TestStaleness_String(t *testing.T) {
	tests := []struct {
		staleness Staleness
		want      string
	}{
		{Staleness{}, "fresh"},
		{Staleness{Reason: NeverRan}, "not run this session"},
		{Staleness{Reason: Expired, Age: 62*time.Minute + 400*time.Millisecond, MaxAge: time.Hour}, "ran 1h2m ago, max_age 1h"},
		{Staleness{Reason: ProducedUnset, Variables: []string{tokenVar}}, "token unset"},
		{Staleness{Reason: InputsChanged, Variables: []string{"username", "password"}}, "inputs changed: username, password"},
		{Staleness{Reason: VariablesMissing, Variables: []string{userIDVar}}, "userId missing"},
	}
	for _, tt := range tests {
		if got := tt.staleness.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
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
	s.succeed(loginPath, time.Minute)
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
	s.succeed(profilePath, time.Minute)
	s.variables[userIDVar] = true
	g := &Graph{Requests: map[string]Node{
		profilePath: {Requires: requires(loginPath), Produces: []string{userIDVar}},
		getUserPath: {Requires: requires(profilePath)},
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
		loginPath:   {Requires: requires(getUserPath)},
		getUserPath: {Requires: requires(loginPath)},
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
	g := &Graph{Requests: map[string]Node{loginPath: {Requires: requires(loginPath)}}}
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
		"A": {Requires: requires("B")},
		"B": {Requires: requires("C")},
		"C": {Requires: requires("B")},
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
		getUserPath: {Requires: requires("Auth/Logn")},
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

func TestStalePrerequisites_ExpiredPrerequisiteRunsAgain(t *testing.T) {
	s := newFakeSession()
	s.succeed(loginPath, 2*time.Hour)
	s.variables[tokenVar] = true
	g := &Graph{Requests: map[string]Node{
		loginPath:   {Produces: []string{tokenVar}},
		getUserPath: {Requires: []Requirement{{Request: loginPath, MaxAge: time.Hour}}},
	}}

	got, err := g.StalePrerequisites(getUserPath, s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{loginPath}; !reflect.DeepEqual(got, want) {
		t.Errorf("StalePrerequisites = %v, want %v", got, want)
	}
}

func TestRefreshPrerequisites(t *testing.T) {
	g := &Graph{Requests: map[string]Node{
		loginPath:   {Requires: requires(profilePath), Produces: []string{tokenVar}},
		getUserPath: {Requires: []Requirement{{Request: loginPath, RefreshOn: []int{401}}, {Request: "Users/List"}}},
	}}
	fresh := newFakeSession()
	for _, path := range []string{loginPath, profilePath, "Users/List"} {
		fresh.succeed(path, time.Minute)
	}
	fresh.variables[tokenVar] = true
	staleProfile := newFakeSession()
	staleProfile.succeed(loginPath, time.Minute)
	staleProfile.variables[tokenVar] = true

	tests := []struct {
		name   string
		status int
		state  fakeSession
		want   []string
	}{
		{"status not listed", 500, fresh, nil},
		{"fresh prerequisite is forced", 401, fresh, []string{loginPath}},
		{"stale prerequisites of the refreshed one come first", 401, staleProfile, []string{profilePath, loginPath}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.RefreshPrerequisites(getUserPath, tt.status, tt.state)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RefreshPrerequisites = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequirementRules(t *testing.T) {
	tests := []struct {
		requirement Requirement
		want        string
	}{
		{Requirement{Request: loginPath}, ""},
		{Requirement{Request: loginPath, MaxAge: time.Hour, RefreshOn: []int{401, 403}}, "max_age 1h, refresh_on 401/403"},
		{Requirement{Request: loginPath, MaxAge: 90 * time.Second}, "max_age 1m30s"},
		{Requirement{Request: loginPath, WhenMissing: []string{tokenVar, userIDVar}}, "when_missing token, userId"},
	}
	for _, tt := range tests {
		if got := tt.requirement.Rules(); got != tt.want {
			t.Errorf("Rules() = %q, want %q", got, tt.want)
		}
	}
}
