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
