package script

import "postOffice/internal/postman"

// RunItemPreRequestScripts runs item's pre-request scripts and writes variable
// changes back to the collection and environment.
func RunItemPreRequestScripts(
	item *postman.Item,
	collection *postman.Collection,
	environment *postman.Environment,
) []string {
	ctx := newItemContext(collection, environment, nil)
	errors := ExecutePreRequestScripts(item.Events, ctx)
	writeBackVariables(ctx, collection, environment)
	return errors
}

// RunItemTestScripts runs item's test scripts against response and writes
// variable changes back to the collection and environment. A nil item yields nil.
func RunItemTestScripts(
	item *postman.Item,
	collection *postman.Collection,
	environment *postman.Environment,
	response *ResponseData,
) *TestResult {
	if item == nil {
		return nil
	}
	ctx := newItemContext(collection, environment, response)
	result := ExecuteTestScripts(item.Events, ctx)
	writeBackVariables(ctx, collection, environment)
	return result
}

func newItemContext(
	collection *postman.Collection,
	environment *postman.Environment,
	response *ResponseData,
) *ExecutionContext {
	ctx := &ExecutionContext{Response: response}
	if collection != nil {
		ctx.CollectionVars = collection.Variables
	}
	if environment != nil {
		ctx.EnvironmentVars = environment.Values
	}
	return ctx
}

func writeBackVariables(ctx *ExecutionContext, collection *postman.Collection, environment *postman.Environment) {
	if collection != nil {
		collection.Variables = ctx.CollectionVars
	}
	if environment != nil {
		environment.Values = ctx.EnvironmentVars
	}
}
