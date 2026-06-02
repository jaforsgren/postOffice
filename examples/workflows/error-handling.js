(function(wf, pm) {
  // Error Handling Example Workflow
  // This demonstrates various error handling patterns and workflow failure

  var maxRetries = 3;
  var retryCount = 0;
  var success = false;

  // Set user ID
  pm.collectionVariables.set("userId", "1");

  // Step 1: Try to fetch user with retry logic
  while (retryCount < maxRetries && !success) {
    var userResult = wf.run("get-user");

    if (userResult.response.code === 200) {
      success = true;
      pm.environment.set("user_fetch_attempts", (retryCount + 1).toString());
    } else if (userResult.response.code === 404) {
      // User not found - this is a permanent failure, don't retry
      wf.fail("User not found (404). Cannot proceed with workflow.");
      return;
    } else if (userResult.response.code >= 500) {
      // Server error - retry
      retryCount++;
      pm.environment.set("last_error", "Server error: " + userResult.response.code);

      if (retryCount >= maxRetries) {
        wf.fail("Max retries exceeded. Last error: " + userResult.response.code);
        return;
      }
    } else {
      // Other client errors
      wf.fail("User fetch failed with unexpected status: " + userResult.response.code);
      return;
    }
  }

  // Step 2: Validate user data before proceeding
  var userData = userResult.response.body;

  // In a real scenario, we'd parse and validate the response
  if (!userData || userData.length === 0) {
    wf.fail("User data is empty or invalid");
    return;
  }

  // Step 3: Try to create a post
  var postResult = wf.run("create-post");

  if (postResult.response.code === 201 || postResult.response.code === 200) {
    // Success
    pm.environment.set("post_created", "true");
    pm.environment.set("workflow_status", "completed");
  } else if (postResult.response.code === 422) {
    // Validation error
    pm.environment.set("post_created", "false");
    pm.environment.set("validation_error", "Post data validation failed");
    wf.fail("Post validation failed. Check the request body.");
  } else if (postResult.response.code === 401 || postResult.response.code === 403) {
    // Authentication/authorization error
    pm.environment.set("post_created", "false");
    wf.fail("Authentication required. Please login first.");
  } else {
    // Other errors
    pm.environment.set("post_created", "false");
    wf.fail("Failed to create post: " + postResult.response.code);
  }

  // Log the final state
  var timestamp = new Date().toISOString();
  pm.environment.set("workflow_completed_at", timestamp);

})(wf, pm);
