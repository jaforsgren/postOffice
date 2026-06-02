(function(wf, pm) {
  // Conditional Post Workflow
  // This demonstrates conditional logic - create or update based on existence

  // Set the post ID we want to check
  var targetPostId = "5";
  pm.collectionVariables.set("postId", targetPostId);
  pm.collectionVariables.set("userId", "1");

  // Step 1: Try to fetch the post
  var checkResult = wf.run("check-post");

  if (checkResult.response.code === 404) {
    // Post doesn't exist - create it
    var createResult = wf.run("create-post");

    if (createResult.response.code === 201 || createResult.response.code === 200) {
      pm.environment.set("operation", "created");
      pm.environment.set("result", "Post created successfully");
    } else {
      wf.fail("Failed to create post");
    }

    // Skip the update step since we just created
    wf.skip("update-post");

  } else if (checkResult.response.code === 200) {
    // Post exists - update it
    wf.skip("create-post");

    var updateResult = wf.run("update-post");

    if (updateResult.response.code === 200) {
      pm.environment.set("operation", "updated");
      pm.environment.set("result", "Post updated successfully");
    } else {
      wf.fail("Failed to update post");
    }

  } else {
    // Unexpected response
    wf.fail("Unexpected response when checking post: " + checkResult.response.code);
  }

  // Log the workflow completion
  var timestamp = new Date().toISOString();
  pm.environment.set("last_workflow_run", timestamp);

})(wf, pm);
