# PostOffice Workflow Examples

This directory contains example collections demonstrating PostOffice workflow capabilities.

## API Testing Workflow Example

The `api-testing-workflow.json` collection demonstrates comprehensive workflow features using the JSONPlaceholder API (a free fake REST API for testing).

### Collection Structure

**Requests:**
- **Authentication** - Login and token validation
- **Users** - Get, list, create user operations
- **Posts** - Full CRUD operations for posts
- **Comments** - Fetch comments for posts

**Workflows:**

1. **Complete User Flow** (`complete-user-flow.js`)
   - Demonstrates a full end-to-end workflow
   - Authentication → User fetch → Post creation → Validation
   - Shows sequential request execution with validation at each step
   - Sets environment variables for workflow state

2. **Conditional Post Workflow** (`conditional-post-workflow.js`)
   - Demonstrates conditional branching logic
   - Checks if a post exists, then creates or updates accordingly
   - Uses `wf.skip()` to skip unnecessary steps
   - Shows if/else branching patterns

3. **Bulk Operations** (`bulk-operations.js`)
   - Demonstrates repeated requests and loops
   - Fetches multiple users in a loop
   - Aggregates success/failure counts
   - Calculates completion rates

4. **Error Handling** (`error-handling.js`)
   - Demonstrates robust error handling patterns
   - Retry logic for transient failures
   - Different handling for different error types (404, 500, 422, 401)
   - Validation before proceeding with workflow steps

## Getting Started

### Load the Collection

1. Start PostOffice:
   ```bash
   ./postOffice
   ```

2. Load the example collection:
   ```
   :load examples/api-testing-workflow.json
   ```

3. (Optional) Load the test environment:
   ```
   :loadenv examples/test-environment.json
   ```

4. Switch to workflows view:
   ```
   :wf
   ```

### Execute Workflows

1. Navigate to a workflow using `j/k`
2. Press `enter` or `ctrl+r` to execute
3. Watch the status in the status bar

### What to Observe

**During Execution:**
- Status messages showing workflow progress
- Step completion counts
- Success/failure indicators

**After Execution:**
- Check environment variables (`:variables`)
- Look for workflow-set variables like:
  - `workflow_completed_at`
  - `workflow_status`
  - `operation` (created/updated)
  - `users_fetched_successfully`
  - `completion_rate`

### Examining Workflow Logic

Each workflow JavaScript file demonstrates different patterns:

**complete-user-flow.js** - Sequential execution:
```javascript
var result = wf.run("step1");
if (result.response.code !== 200) {
  wf.fail("Step 1 failed");
  return;
}
// Continue to next step...
```

**conditional-post-workflow.js** - Branching:
```javascript
if (result.response.code === 404) {
  wf.run("create-post");
  wf.skip("update-post");
} else {
  wf.skip("create-post");
  wf.run("update-post");
}
```

**bulk-operations.js** - Loops:
```javascript
for (var i = 0; i < userIds.length; i++) {
  pm.collectionVariables.set("userId", userIds[i]);
  var result = wf.run("get-user");
  // Process result...
}
```

**error-handling.js** - Retry logic:
```javascript
while (retryCount < maxRetries && !success) {
  var result = wf.run("get-user");
  if (result.response.code === 200) {
    success = true;
  } else if (result.response.code >= 500) {
    retryCount++;
  }
}
```

## Modifying Workflows

### Edit a Workflow

1. Open the JavaScript file in `examples/workflows/`
2. Modify the logic
3. Save the file
4. Re-execute the workflow in PostOffice (no reload needed)

### Create a New Workflow

1. In PostOffice, create a new workflow:
   ```
   :wf new my-custom-workflow
   ```

2. Edit `workflows/my-custom-workflow.js`

3. Add workflow steps to the collection JSON:
   ```json
   "my-custom-workflow": {
     "name": "My Custom Workflow",
     "entry": "workflows/my-custom-workflow.js",
     "version": 1,
     "steps": [
       {"id": "step1", "request": "Users/Get User", "repeat": 1}
     ]
   }
   ```

## Best Practices Demonstrated

1. **Early Validation** - Check responses before proceeding
2. **Error Context** - Set environment variables to track state
3. **Fail Fast** - Use `wf.fail()` with descriptive messages
4. **Conditional Logic** - Use `wf.skip()` for unused branches
5. **Data Flow** - Use collection/environment variables to pass data between steps
6. **Retry Logic** - Handle transient failures gracefully
7. **Status Tracking** - Set completion timestamps and status flags

## Troubleshooting

**Workflow fails immediately:**
- Check that the base URL in collection variables is correct
- Ensure the workflow JavaScript file exists in the correct path

**Steps are skipped:**
- This is intentional when using `wf.skip()` in conditional logic
- Check the workflow logic to see why steps were skipped

**Variables not updating:**
- Make sure you're setting variables on the correct scope (environment vs collection)
- Use `:variables` to view all variables and their sources

## Further Learning

- Explore the workflow JavaScript files to see implementation details
- Modify the workflows to add your own logic
- Create new workflows combining different patterns
- Check the main README.md for complete API reference
