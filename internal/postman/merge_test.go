package postman

import "testing"

func TestMergeCollections_AppendsUniqueItems(t *testing.T) {
	dst := &Collection{
		Info:  Info{Name: "Dst"},
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
	}
	src := &Collection{
		Info:  Info{Name: "Src"},
		Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}},
	}

	merged := MergeCollections(dst, src, MergeKeepOriginal)

	if merged.Info.Name != "Dst" {
		t.Errorf("expected merged Info.Name to be preserved as %q, got %q", "Dst", merged.Info.Name)
	}
	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(merged.Items))
	}
	if merged.Items[0].Name != "Get Users" || merged.Items[1].Name != "Get Orders" {
		t.Errorf("expected items [Get Users, Get Orders], got [%s, %s]", merged.Items[0].Name, merged.Items[1].Name)
	}
}

func TestMergeCollections_RequestConflict(t *testing.T) {
	dst := &Collection{
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
	}
	src := &Collection{
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "POST"}}},
	}

	t.Run("keep-original keeps dst request", func(t *testing.T) {
		merged := MergeCollections(dst, src, MergeKeepOriginal)
		if merged.Items[0].Request.Method != "GET" {
			t.Errorf("expected method GET, got %s", merged.Items[0].Request.Method)
		}
	})

	t.Run("forceMerge takes src request", func(t *testing.T) {
		merged := MergeCollections(dst, src, MergeForce)
		if merged.Items[0].Request.Method != "POST" {
			t.Errorf("expected method POST, got %s", merged.Items[0].Request.Method)
		}
	})
}

func TestMergeCollections_FoldersMergeRecursively(t *testing.T) {
	dst := &Collection{
		Items: []Item{
			{
				Name: "Users",
				Items: []Item{
					{Name: "Get User", Request: &Request{Method: "GET"}},
				},
			},
		},
	}
	src := &Collection{
		Items: []Item{
			{
				Name: "Users",
				Items: []Item{
					{Name: "Delete User", Request: &Request{Method: "DELETE"}},
				},
			},
		},
	}

	merged := MergeCollections(dst, src, MergeKeepOriginal)

	if len(merged.Items) != 1 {
		t.Fatalf("expected folders with the same name to merge into one item, got %d", len(merged.Items))
	}
	folder := merged.Items[0]
	if !folder.IsFolder() {
		t.Fatalf("expected merged item to remain a folder")
	}
	if len(folder.Items) != 2 {
		t.Fatalf("expected folder children to merge to 2 items, got %d", len(folder.Items))
	}
}

func TestMergeCollections_FolderVsRequestTypeConflict(t *testing.T) {
	dst := &Collection{
		Items: []Item{{Name: "Users", Items: []Item{{Name: "Get", Request: &Request{Method: "GET"}}}}},
	}
	src := &Collection{
		Items: []Item{{Name: "Users", Request: &Request{Method: "POST"}}},
	}

	merged := MergeCollections(dst, src, MergeForce)

	if merged.Items[0].IsFolder() {
		t.Errorf("expected forceMerge to replace folder with src's request item")
	}
	if merged.Items[0].Request == nil || merged.Items[0].Request.Method != "POST" {
		t.Errorf("expected merged item to be src's POST request")
	}
}

func TestMergeCollections_Variables(t *testing.T) {
	dst := &Collection{
		Variables: []Variable{{Key: "baseUrl", Value: "https://dst.example.com"}},
	}
	src := &Collection{
		Variables: []Variable{
			{Key: "baseUrl", Value: "https://src.example.com"},
			{Key: "apiKey", Value: "secret"},
		},
	}

	merged := MergeCollections(dst, src, MergeForce)

	if len(merged.Variables) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(merged.Variables))
	}
	if merged.Variables[0].Value != "https://src.example.com" {
		t.Errorf("expected forceMerge to overwrite baseUrl, got %s", merged.Variables[0].Value)
	}
	if merged.Variables[1].Key != "apiKey" {
		t.Errorf("expected apiKey to be appended, got %s", merged.Variables[1].Key)
	}
}

func TestMergeCollections_EventsDeduped(t *testing.T) {
	dst := &Collection{
		Events: []Event{{Listen: "prerequest", Script: Script{Exec: []string{"console.log('dst')"}}}},
	}
	src := &Collection{
		Events: []Event{
			{Listen: "prerequest", Script: Script{Exec: []string{"console.log('dst')"}}},
			{Listen: "test", Script: Script{Exec: []string{"pm.test(...)"}}},
		},
	}

	merged := MergeCollections(dst, src, MergeKeepOriginal)

	if len(merged.Events) != 2 {
		t.Fatalf("expected duplicate event to be deduped and unique one appended, got %d events", len(merged.Events))
	}
}

func TestMergeEnvironments(t *testing.T) {
	dst := &Environment{
		ID:   "dst-id",
		Name: "Dst Env",
		Values: []EnvVariable{
			{Key: "host", Value: "dst-host", Enabled: true},
		},
	}
	src := &Environment{
		ID:   "src-id",
		Name: "Src Env",
		Values: []EnvVariable{
			{Key: "host", Value: "src-host", Enabled: true},
			{Key: "token", Value: "abc123", Enabled: true},
		},
	}

	t.Run("keep-original preserves dst id, name and conflicting value", func(t *testing.T) {
		merged := MergeEnvironments(dst, src, MergeKeepOriginal)
		if merged.ID != "dst-id" || merged.Name != "Dst Env" {
			t.Errorf("expected dst ID/Name to be preserved, got %s/%s", merged.ID, merged.Name)
		}
		if len(merged.Values) != 2 {
			t.Fatalf("expected 2 values, got %d", len(merged.Values))
		}
		if merged.Values[0].Value != "dst-host" {
			t.Errorf("expected keep-original to keep dst-host, got %s", merged.Values[0].Value)
		}
	})

	t.Run("forceMerge overwrites conflicting value and keeps dst id/name", func(t *testing.T) {
		merged := MergeEnvironments(dst, src, MergeForce)
		if merged.ID != "dst-id" || merged.Name != "Dst Env" {
			t.Errorf("expected dst ID/Name to be preserved, got %s/%s", merged.ID, merged.Name)
		}
		if merged.Values[0].Value != "src-host" {
			t.Errorf("expected forceMerge to overwrite with src-host, got %s", merged.Values[0].Value)
		}
		if merged.Values[1].Key != "token" {
			t.Errorf("expected token to be appended, got %s", merged.Values[1].Key)
		}
	})
}

func TestMergeCollectionsGrouped_NestsUnderCollectionNames(t *testing.T) {
	dst := &Collection{
		Info:  Info{Name: "Dst Collection"},
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
	}
	src := &Collection{
		Info:  Info{Name: "Src Collection"},
		Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}},
	}

	merged := MergeCollectionsGrouped(dst, src, "Dst Collection", "Src Collection", MergeKeepOriginal)

	if merged.Info.Name != "Dst Collection" {
		t.Errorf("expected merged Info.Name to be preserved, got %q", merged.Info.Name)
	}
	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 top-level group folders, got %d", len(merged.Items))
	}
	if merged.Items[0].Name != "Dst Collection" || !merged.Items[0].IsFolder() {
		t.Errorf("expected first group folder %q, got %+v", "Dst Collection", merged.Items[0])
	}
	if merged.Items[1].Name != "Src Collection" || !merged.Items[1].IsFolder() {
		t.Errorf("expected second group folder %q, got %+v", "Src Collection", merged.Items[1])
	}
	if len(merged.Items[0].Items) != 1 || merged.Items[0].Items[0].Name != "Get Users" {
		t.Errorf("expected Dst Collection folder to contain Get Users, got %+v", merged.Items[0].Items)
	}
	if len(merged.Items[1].Items) != 1 || merged.Items[1].Items[0].Name != "Get Orders" {
		t.Errorf("expected Src Collection folder to contain Get Orders, got %+v", merged.Items[1].Items)
	}
}

func TestMergeCollectionsGrouped_SameNameMergesIntoOneFolderWithOverwrite(t *testing.T) {
	dst := &Collection{
		Info: Info{Name: "Shared Name"},
		Items: []Item{
			{Name: "Get Users", Request: &Request{Method: "GET"}},
		},
	}
	src := &Collection{
		Info: Info{Name: "Shared Name"},
		Items: []Item{
			{Name: "Get Users", Request: &Request{Method: "POST"}},
			{Name: "Get Orders", Request: &Request{Method: "GET"}},
		},
	}

	t.Run("forceMerge overwrites the clashing request inside the shared folder", func(t *testing.T) {
		merged := MergeCollectionsGrouped(dst, src, "Shared Name", "Shared Name", MergeForce)

		if len(merged.Items) != 1 {
			t.Fatalf("expected a single merged group folder, got %d", len(merged.Items))
		}
		folder := merged.Items[0]
		if folder.Name != "Shared Name" {
			t.Errorf("expected folder name Shared Name, got %s", folder.Name)
		}
		if len(folder.Items) != 2 {
			t.Fatalf("expected 2 items inside the merged folder, got %d", len(folder.Items))
		}
		if folder.Items[0].Request.Method != "POST" {
			t.Errorf("expected forceMerge to overwrite Get Users with POST, got %s", folder.Items[0].Request.Method)
		}
	})

	t.Run("keep-original keeps the clashing request inside the shared folder", func(t *testing.T) {
		merged := MergeCollectionsGrouped(dst, src, "Shared Name", "Shared Name", MergeKeepOriginal)

		folder := merged.Items[0]
		if folder.Items[0].Request.Method != "GET" {
			t.Errorf("expected keep-original to keep Get Users as GET, got %s", folder.Items[0].Request.Method)
		}
	})
}

func TestMergeCollectionsGrouped_ReusesExistingTopLevelGroupFolder(t *testing.T) {
	dst := &Collection{
		Info: Info{Name: "My Collection"},
		Items: []Item{
			{
				Name:  "My Collection",
				Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
			},
		},
	}
	src := &Collection{
		Info:  Info{Name: "Other Collection"},
		Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}},
	}

	merged := MergeCollectionsGrouped(dst, src, "My Collection", "Other Collection", MergeKeepOriginal)

	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 top-level items (existing My Collection folder + new Other Collection folder), got %d", len(merged.Items))
	}
	if merged.Items[0].Name != "My Collection" {
		t.Errorf("expected existing group folder to be reused as-is, got %+v", merged.Items[0])
	}
	// It must not have been double-wrapped into My Collection > My Collection > Get Users.
	if len(merged.Items[0].Items) != 1 || merged.Items[0].Items[0].Name != "Get Users" {
		t.Errorf("expected existing folder contents untouched, got %+v", merged.Items[0].Items)
	}
	if merged.Items[1].Name != "Other Collection" {
		t.Errorf("expected new group folder for Other Collection, got %+v", merged.Items[1])
	}
}

func TestMergeCollectionsGrouped_ReusedFoldersOnBothSidesMergeTogether(t *testing.T) {
	dst := &Collection{
		Info: Info{Name: "Shared"},
		Items: []Item{
			{Name: "Shared", Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}}},
		},
	}
	src := &Collection{
		Info: Info{Name: "Shared"},
		Items: []Item{
			{Name: "Shared", Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}}},
		},
	}

	merged := MergeCollectionsGrouped(dst, src, "Shared", "Shared", MergeKeepOriginal)

	if len(merged.Items) != 1 {
		t.Fatalf("expected the two existing 'Shared' folders to merge into one, got %d top-level items", len(merged.Items))
	}
	folder := merged.Items[0]
	if len(folder.Items) != 2 {
		t.Fatalf("expected merged folder to contain both requests, got %+v", folder.Items)
	}
	names := []string{folder.Items[0].Name, folder.Items[1].Name}
	if names[0] != "Get Users" || names[1] != "Get Orders" {
		t.Errorf("expected [Get Users, Get Orders], got %v", names)
	}
}

func TestMergeCollectionsGrouped_GroupFoldersCarryCollectionDocumentation(t *testing.T) {
	dst := &Collection{Info: Info{Name: "Dst", Description: "dst docs"}, Items: []Item{{Name: "a", Request: &Request{Method: "GET"}}}}
	src := &Collection{Info: Info{Name: "Src", Description: "src docs"}, Items: []Item{{Name: "b", Request: &Request{Method: "GET"}}}}

	merged := MergeCollectionsGrouped(dst, src, "Dst", "Src", MergeKeepOriginal)

	if merged.Items[0].Description != "dst docs" || merged.Items[1].Description != "src docs" {
		t.Errorf("expected each group folder documented like its collection, got %+v", merged.Items)
	}
}
