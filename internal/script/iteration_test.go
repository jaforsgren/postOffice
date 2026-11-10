package script

import (
	"testing"

	"postOffice/internal/postman"
)

func TestPmInfo_IterationDefaultsToASinglePass(t *testing.T) {
	result := runPreRequestScript(t, &ExecutionContext{},
		"pm.test('iteration', () => pm.expect(pm.info.iteration).to.eql(0));",
		"pm.test('count', () => pm.expect(pm.info.iterationCount).to.eql(1));",
	)
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestPmIterationData(t *testing.T) {
	ctx := &ExecutionContext{
		Iteration:       Iteration{Index: 2, Count: 5, Data: IterationData{"user": "alice", "age": float64(30), "tags": []any{"a"}}},
		EnvironmentVars: []postman.EnvVariable{{Key: "user", Value: "from-env"}},
	}

	result := runPreRequestScript(t, ctx,
		"pm.test('info iteration', () => pm.expect(pm.info.iteration).to.eql(2));",
		"pm.test('info count', () => pm.expect(pm.info.iterationCount).to.eql(5));",
		"pm.test('get keeps types', () => pm.expect(pm.iterationData.get('age')).to.eql(30));",
		"pm.test('get missing', () => pm.expect(pm.iterationData.get('nope')).to.be.undefined);",
		"pm.test('has', () => pm.expect(pm.iterationData.has('user')).to.be.true);",
		"pm.test('toObject', () => pm.expect(pm.iterationData.toObject()).to.have.keys('user', 'age', 'tags'));",
		"pm.test('variables prefer data', () => pm.expect(pm.variables.get('user')).to.eql('alice'));",
		"pm.test('environment untouched', () => pm.expect(pm.environment.get('user')).to.eql('from-env'));",
		"pm.test('non-string data as JSON text', () => pm.expect(pm.variables.get('tags')).to.eql('[\"a\"]'));",
	)
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestPmIterationData_EmptyWithoutDataFile(t *testing.T) {
	result := runPreRequestScript(t, &ExecutionContext{},
		"pm.test('has', () => pm.expect(pm.iterationData.has('user')).to.be.false);",
		"pm.test('toObject', () => pm.expect(pm.iterationData.toObject()).to.be.empty);",
	)
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestRequestScopeVariables_DataAboveEnvironment(t *testing.T) {
	scope := RequestScope{
		Collection:  &postman.Collection{Variables: []postman.Variable{{Key: "base", Value: "collection"}, {Key: "user", Value: "collection"}}},
		Environment: &postman.Environment{Name: "dev", Values: []postman.EnvVariable{{Key: "user", Value: "env", Enabled: true}}},
		Iteration:   Iteration{Data: IterationData{"user": "alice", "id": float64(7)}},
	}

	resolved := postman.ResolveVariables("{{base}}/{{user}}/{{id}}", scope.Variables())

	if resolved != "collection/alice/7" {
		t.Errorf("expected data > environment > collection, got %q", resolved)
	}
}
