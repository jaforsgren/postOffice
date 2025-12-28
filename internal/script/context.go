package script

import (
	"postOffice/internal/postman"
)

type ResponseData struct {
	StatusCode   int
	Status       string
	Body         string
	Headers      map[string][]string
	ResponseTime int64
}

type ExecutionContext struct {
	Response        *ResponseData
	CollectionVars  []postman.Variable
	EnvironmentVars []postman.EnvVariable
}

type TestResult struct {
	Tests  []Test
	Errors []string
}

type Test struct {
	Name   string
	Passed bool
	Error  string
}

func (tr *TestResult) AddTest(name string, passed bool, err string) {
	tr.Tests = append(tr.Tests, Test{
		Name:   name,
		Passed: passed,
		Error:  err,
	})
}

func (tr *TestResult) AddError(err string) {
	tr.Errors = append(tr.Errors, err)
}

func (tr *TestResult) HasFailures() bool {
	for _, test := range tr.Tests {
		if !test.Passed {
			return true
		}
	}
	return len(tr.Errors) > 0
}
