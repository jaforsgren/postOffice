package script

import (
	"strings"
	"testing"
)

func runExpect(t *testing.T, assertion string) Test {
	t.Helper()
	result := NewRuntime().ExecuteTestScript(
		jsScript("pm.test('assertion', () => { "+assertion+"; });"),
		&ExecutionContext{Response: &ResponseData{StatusCode: 200, Body: `{"items":[1,2],"name":"sapi"}`}},
	)
	if len(result.Errors) > 0 {
		t.Fatalf("%s: unexpected script errors: %v", assertion, result.Errors)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("%s: expected 1 test, got %d", assertion, len(result.Tests))
	}
	return result.Tests[0]
}

func TestPmExpect_Passing(t *testing.T) {
	assertions := []string{
		"pm.expect({ a: [1, { b: 2 }] }).to.eql({ a: [1, { b: 2 }] })",
		"pm.expect([1, 2]).to.not.eql([2, 1])",
		"pm.expect({ a: 1 }).not.to.eql({ a: 2 })",
		"pm.expect(3).to.equal(3)",
		"pm.expect({ a: 1 }).to.deep.equal({ a: 1 })",
		"pm.expect(201).to.be.oneOf([200, 201])",
		"pm.expect('x').to.be.a('string')",
		"pm.expect([]).to.be.an('array')",
		"pm.expect(null).to.be.a('null')",
		"pm.expect({}).to.be.an('object')",
		"pm.expect('hello world').to.include('world')",
		"pm.expect([1, 2, 3]).to.include(2)",
		"pm.expect({ a: 1, b: 2 }).to.include({ a: 1 })",
		"pm.expect([{ id: 1 }]).to.deep.include({ id: 1 })",
		"pm.expect('abc').to.not.include('z')",
		"pm.expect([1]).not.to.include(2)",
		"pm.expect({ type: 1, title: 2, status: 3, detail: 4 }).to.include.keys('type', 'title', 'status')",
		"pm.expect({ a: 1, b: 2 }).to.have.keys('a', 'b')",
		"pm.expect({ a: 1, b: 2 }).to.have.any.keys('a', 'z')",
		"pm.expect(5).to.be.above(4)",
		"pm.expect(4).to.be.below(5)",
		"pm.expect(5).to.be.at.most(5)",
		"pm.expect(5).to.be.at.least(5)",
		"pm.expect(5).to.be.within(1, 10)",
		"pm.expect(true).to.be.true",
		"pm.expect(false).to.be.false",
		"pm.expect(null).to.be.null",
		"pm.expect(undefined).to.be.undefined",
		"pm.expect(1).to.exist",
		"pm.expect('').to.be.empty",
		"pm.expect([]).to.be.empty",
		"pm.expect({}).to.be.empty",
		"pm.expect([1]).to.not.be.empty",
		"pm.expect([1, 2]).to.have.lengthOf(2)",
		"pm.expect('abc').to.have.length(3)",
		"pm.expect([1, 2, 3]).to.have.length.above(2)",
		"pm.expect('2026-11-10').to.match(/^\\d{4}-\\d{2}-\\d{2}$/)",
		"pm.expect({ id: 'x' }).to.have.property('id')",
		"pm.expect({ id: 'x' }).to.have.property('id', 'x')",
		"pm.expect({ id: 'x' }).to.have.property('id').that.is.a('string')",
		"pm.expect({ a: 1 }).to.not.have.property('b')",
		"pm.expect([3, 1, 2]).to.have.members([1, 2, 3])",
		"pm.expect([1, 2, 3]).to.include.members([1, 2])",
		"pm.expect(1).to.be.ok",
		"pm.expect(pm.response.json().items).to.eql([1, 2])",
		"pm.expect(pm.response.json()).to.have.property('name', 'sapi')",
		"pm.expect(() => { throw new Error('boom'); }).to.throw(/boom/)",
	}

	for _, assertion := range assertions {
		if test := runExpect(t, assertion); !test.Passed {
			t.Errorf("%s: expected to pass, failed with %q", assertion, test.Error)
		}
	}
}

func TestPmExpect_Failing(t *testing.T) {
	cases := []struct {
		assertion string
		message   string
	}{
		{"pm.expect(200).to.eql(201)", "expected 200 to deeply equal 201"},
		{"pm.expect({ a: 1 }).to.eql({ a: 2 })", "expected { a: 1 } to deeply equal { a: 2 }"},
		{"pm.expect(1).to.not.eql(1)", "expected 1 to not deeply equal 1"},
		{"pm.expect(500).to.be.oneOf([200, 201])", "expected 500 to be one of [ 200, 201 ]"},
		{"pm.expect(1).to.be.a('string')", "expected 1 to be a string"},
		{"pm.expect('x').to.be.an('array')", "expected 'x' to be an array"},
		{"pm.expect('abc').to.include('z')", "expected 'abc' to include 'z'"},
		{"pm.expect('abc').to.not.include('a')", "expected 'abc' to not include 'a'"},
		{"pm.expect({ a: 1 }).to.include.keys('a', 'b')", "to have all of keys [ 'a', 'b' ]"},
		{"pm.expect(10001).to.be.below(10000)", "expected 10001 to be below 10000"},
		{"pm.expect(6).to.be.at.most(5)", "expected 6 to be at most 5"},
		{"pm.expect(11).to.be.within(1, 10)", "expected 11 to be within 1..10"},
		{"pm.expect(1).to.be.true", "expected 1 to be true"},
		{"pm.expect(0).to.be.undefined", "expected 0 to be undefined"},
		{"pm.expect([1]).to.be.empty", "expected [ 1 ] to be empty"},
		{"pm.expect([]).to.not.be.empty", "expected [] not to be empty"},
		{"pm.expect([1]).to.have.lengthOf(2)", "to have a length of 2 but got 1"},
		{"pm.expect('x').to.match(/y/)", "expected 'x' to match /y/"},
		{"pm.expect({}).to.have.property('id')", "expected {} to have property 'id'"},
		{"pm.expect([1, 2]).to.have.members([1, 3])", "to have the same members as [ 1, 3 ]"},
		{"pm.expect(1, 'custom context').to.eql(2)", "custom context: expected 1 to deeply equal 2"},
		{"pm.expect.fail('still not ready after 10 attempts')", "still not ready after 10 attempts"},
	}

	for _, tc := range cases {
		test := runExpect(t, tc.assertion)
		if test.Passed {
			t.Errorf("%s: expected to fail", tc.assertion)
			continue
		}
		if !strings.HasPrefix(test.Error, "AssertionError: ") || !strings.Contains(test.Error, tc.message) {
			t.Errorf("%s: expected AssertionError containing %q, got %q", tc.assertion, tc.message, test.Error)
		}
	}
}

func TestPmExpect_OutsidePmTestIsAScriptError(t *testing.T) {
	result := NewRuntime().ExecuteTestScript(jsScript("pm.expect(1).to.eql(2);"), &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "expected 1 to deeply equal 2") {
		t.Errorf("expected the assertion to surface as a script error, got %v", result.Errors)
	}
}
