(function(wf, pm) {
  // Complete User Flow Workflow
  // This demonstrates a full end-to-end workflow with authentication,
  // data fetching, resource creation, and validation

  // Set initial user ID
  pm.collectionVariables.set("userId", "1");

  // Step 1: Authenticate
  var loginResult = wf.run("login");

  if (loginResult.response.code !== 200) {
    wf.fail("Login failed with status: " + loginResult.response.code);
    return;
  }

  // Simulate extracting auth token from response
  var authToken = "mock-token-" + Date.now();
  pm.collectionVariables.set("authToken", authToken);

  // Step 2: Validate the token
  var validateResult = wf.run("validate-token");

  if (validateResult.response.code !== 200) {
    wf.fail("Token validation failed");
    return;
  }

  // Step 3: Fetch user data
  var userResult = wf.run("get-user");

  if (userResult.response.code !== 200) {
    wf.fail("Failed to fetch user data");
    return;
  }

  // Step 4: Create a new post
  var createPostResult = wf.run("create-post");

  if (createPostResult.response.code !== 201 && createPostResult.response.code !== 200) {
    wf.fail("Failed to create post");
    return;
  }

  // Extract post ID from response (simulated for JSONPlaceholder)
  pm.collectionVariables.set("postId", "1");

  // Step 5: Verify the post was created
  var getPostResult = wf.run("get-post");

  if (getPostResult.response.code !== 200) {
    wf.fail("Failed to fetch created post");
    return;
  }

  // Step 6: Fetch comments for the post
  var commentsResult = wf.run("get-comments");

  if (commentsResult.response.code === 200) {
    // Success! Set a completion timestamp
    pm.environment.set("workflow_completed_at", new Date().toISOString());
    pm.environment.set("workflow_status", "success");
  } else {
    wf.fail("Failed to fetch comments");
  }

})(wf, pm);
