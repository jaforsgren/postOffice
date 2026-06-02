(function(wf, pm) {
  // Bulk Operations Workflow
  // This demonstrates repeated requests and data aggregation

  var successCount = 0;
  var failureCount = 0;
  var userIds = [];

  // Step 1: Fetch all users
  var listUsersResult = wf.run("list-users");

  if (listUsersResult.response.code !== 200) {
    wf.fail("Failed to fetch user list");
    return;
  }

  // In a real scenario, we'd parse the response and extract user IDs
  // For this example, we'll simulate it
  userIds = ["1", "2", "3", "4", "5"];
  pm.environment.set("total_users", userIds.length.toString());

  // Step 2: Fetch each user individually (demonstrating repeated requests)
  for (var i = 0; i < userIds.length; i++) {
    pm.collectionVariables.set("userId", userIds[i]);

    var userResult = wf.run("get-user");

    if (userResult.response.code === 200) {
      successCount++;
    } else {
      failureCount++;
    }
  }

  // Store the results
  pm.environment.set("users_fetched_successfully", successCount.toString());
  pm.environment.set("users_fetch_failed", failureCount.toString());

  // Step 3: Fetch all posts
  var listPostsResult = wf.run("list-posts");

  if (listPostsResult.response.code !== 200) {
    wf.fail("Failed to fetch posts list");
    return;
  }

  // Calculate completion percentage
  var completionRate = (successCount / userIds.length) * 100;
  pm.environment.set("completion_rate", completionRate.toFixed(2) + "%");

  // Set workflow summary
  var summary = "Processed " + userIds.length + " users. " +
                "Success: " + successCount + ", " +
                "Failures: " + failureCount;

  pm.environment.set("workflow_summary", summary);

  // Only fail if all requests failed
  if (failureCount === userIds.length) {
    wf.fail("All user fetch requests failed");
  }

})(wf, pm);
