package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestToggleActiveCollection(t *testing.T) {
	p := NewParser()

	p.ToggleActiveCollection("A")
	p.ToggleActiveCollection("B")

	if !p.IsCollectionActive("A") || !p.IsCollectionActive("B") {
		t.Fatalf("expected A and B to be active, got %v", p.ActiveCollections())
	}
	if p.ActiveCollectionCount() != 2 {
		t.Errorf("expected 2 active collections, got %d", p.ActiveCollectionCount())
	}

	p.ToggleActiveCollection("A")

	if p.IsCollectionActive("A") {
		t.Error("expected A to be inactive after second toggle")
	}
	if got := p.ActiveCollections(); len(got) != 1 || got[0] != "B" {
		t.Errorf("expected only B active, got %v", got)
	}
}

func TestToggleActiveEnvironment(t *testing.T) {
	p := NewParser()

	p.ToggleActiveEnvironment("Dev")
	if !p.IsEnvironmentActive("Dev") {
		t.Fatal("expected Dev to be active")
	}

	p.ToggleActiveEnvironment("Dev")
	if p.IsEnvironmentActive("Dev") {
		t.Error("expected Dev to be inactive after second toggle")
	}
	if p.ActiveEnvironmentCount() != 0 {
		t.Errorf("expected 0 active environments, got %d", p.ActiveEnvironmentCount())
	}
}

func TestMergedCollection_NoneActive(t *testing.T) {
	p := NewParser()
	if merged := p.MergedCollection(); merged != nil {
		t.Errorf("expected nil merged collection when none active, got %+v", merged)
	}
}

func TestMergedCollection_GroupsUnderTopLevelFolders(t *testing.T) {
	p := NewParser()
	pathA := createTempCollection(t, &Collection{
		Info:  Info{Name: "Collection A"},
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
	})
	pathB := createTempCollection(t, &Collection{
		Info:  Info{Name: "Collection B"},
		Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}},
	})
	p.LoadCollection(pathA)
	p.LoadCollection(pathB)

	p.ToggleActiveCollection("Collection A")
	p.ToggleActiveCollection("Collection B")

	merged := p.MergedCollection()
	if merged == nil {
		t.Fatal("expected merged collection")
	}
	if len(merged.Items) != 2 {
		t.Fatalf("expected 2 top-level group folders, got %d", len(merged.Items))
	}
	if merged.Items[0].Name != "Collection A" || !merged.Items[0].IsFolder() {
		t.Errorf("expected first group folder named 'Collection A', got %+v", merged.Items[0])
	}
	if merged.Items[1].Name != "Collection B" || !merged.Items[1].IsFolder() {
		t.Errorf("expected second group folder named 'Collection B', got %+v", merged.Items[1])
	}
	if len(merged.Items[0].Items) != 1 || merged.Items[0].Items[0].Name != "Get Users" {
		t.Errorf("expected Collection A's items nested under its group folder, got %+v", merged.Items[0].Items)
	}

	// The merged Info.Name must not collide with any real collection name,
	// so callers can use pathMap lookups to detect and block edits on it.
	if _, exists := p.GetCollectionPath(merged.Info.Name); exists {
		t.Errorf("merged collection name %q unexpectedly resolves to a real path", merged.Info.Name)
	}
}

func TestMergedCollection_SkipsInactiveNames(t *testing.T) {
	p := NewParser()
	p.ToggleActiveCollection("Never Loaded")

	if merged := p.MergedCollection(); merged != nil {
		t.Errorf("expected nil merged collection for an active name with no backing collection, got %+v", merged)
	}
}

func TestMergedEnvironment_NoneActive(t *testing.T) {
	p := NewParser()
	if merged := p.MergedEnvironment(); merged != nil {
		t.Errorf("expected nil merged environment when none active, got %+v", merged)
	}
}

func TestMergedEnvironment_FirstActivatedTakesPrecedence(t *testing.T) {
	p := NewParser()
	pathDev := createTempEnvironment(t, &Environment{
		Name: "Dev",
		Values: []EnvVariable{
			{Key: "host", Value: "dev.example.com", Enabled: true},
			{Key: "only_dev", Value: "1", Enabled: true},
		},
	})
	pathShared := createTempEnvironment(t, &Environment{
		Name: "Shared",
		Values: []EnvVariable{
			{Key: "host", Value: "shared.example.com", Enabled: true},
			{Key: "only_shared", Value: "1", Enabled: true},
		},
	})
	p.LoadEnvironment(pathDev)
	p.LoadEnvironment(pathShared)

	p.ToggleActiveEnvironment("Dev")
	p.ToggleActiveEnvironment("Shared")

	merged := p.MergedEnvironment()
	if merged == nil {
		t.Fatal("expected merged environment")
	}
	if merged.Name != "Dev + Shared" {
		t.Errorf("expected merged name 'Dev + Shared', got %q", merged.Name)
	}

	values := make(map[string]string)
	for _, v := range merged.Values {
		values[v.Key] = v.Value
	}
	if values["host"] != "dev.example.com" {
		t.Errorf("expected first-activated environment to win on conflict, got host=%q", values["host"])
	}
	if values["only_dev"] != "1" || values["only_shared"] != "1" {
		t.Errorf("expected non-conflicting keys from both environments, got %+v", values)
	}

	if _, exists := p.GetEnvironmentPath(merged.Name); exists {
		t.Errorf("merged environment name %q unexpectedly resolves to a real path", merged.Name)
	}
}

func TestActiveSets_PersistAcrossSaveAndLoadState(t *testing.T) {
	parser1 := NewParser()
	pathA := createTempCollection(t, &Collection{Info: Info{Name: "Active Collection A"}})
	pathB := createTempCollection(t, &Collection{Info: Info{Name: "Active Collection B"}})
	envPath := createTempEnvironment(t, &Environment{Name: "Active Env"})

	parser1.LoadCollection(pathA)
	parser1.LoadCollection(pathB)
	parser1.LoadEnvironment(envPath)

	parser1.ToggleActiveCollection("Active Collection A")
	parser1.ToggleActiveCollection("Active Collection B")
	parser1.ToggleActiveEnvironment("Active Env")

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if err := parser1.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	parser2 := NewParser()
	if err := parser2.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if parser2.ActiveCollectionCount() != 2 {
		t.Errorf("expected 2 active collections restored, got %d", parser2.ActiveCollectionCount())
	}
	if !parser2.IsCollectionActive("Active Collection A") || !parser2.IsCollectionActive("Active Collection B") {
		t.Errorf("expected both collections active after restore, got %v", parser2.ActiveCollections())
	}
	if !parser2.IsEnvironmentActive("Active Env") {
		t.Errorf("expected Active Env to be active after restore, got %v", parser2.ActiveEnvironments())
	}
}

func TestActiveSets_LoadStateDropsNamesThatFailedToLoad(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	config := PersistenceConfig{
		CollectionPaths:       []string{},
		EnvironmentPaths:      []string{},
		ActiveCollectionNames: []string{"Ghost Collection"},
	}
	configPath := filepath.Join(tmpHome, configFileName)
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	parser := NewParser()
	if err := parser.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if parser.ActiveCollectionCount() != 0 {
		t.Errorf("expected active names for collections that never loaded to be dropped, got %v", parser.ActiveCollections())
	}
}

func mergedTestParser(t *testing.T) *Parser {
	t.Helper()
	p := NewParser()
	p.LoadCollection(createTempCollection(t, &Collection{
		Info:  Info{Name: "Collection A"},
		Items: []Item{{Name: "Get Users", Request: &Request{Method: "GET"}}},
	}))
	p.LoadCollection(createTempCollection(t, &Collection{
		Info:  Info{Name: "Collection B"},
		Items: []Item{{Name: "Get Orders", Request: &Request{Method: "GET"}}},
	}))
	p.ToggleActiveCollection("Collection A")
	p.ToggleActiveCollection("Collection B")
	return p
}

func TestSyncMergedCollection_WritesTopLevelChangesBack(t *testing.T) {
	p := mergedTestParser(t)
	merged := p.MergedCollection()
	merged.Items[0].Items = append(merged.Items[0].Items, Item{Name: "Create User", Request: &Request{Method: "POST"}})
	merged.Items[1].Items = nil

	p.SyncMergedCollection(merged)

	a, _ := p.GetCollection("Collection A")
	b, _ := p.GetCollection("Collection B")
	if len(a.Items) != 2 || a.Items[1].Name != "Create User" {
		t.Errorf("expected the added request in Collection A, got %+v", a.Items)
	}
	if len(b.Items) != 0 {
		t.Errorf("expected Collection B's removed request gone, got %+v", b.Items)
	}
}

func TestSyncMergedCollection_IgnoresInactiveCollections(t *testing.T) {
	p := mergedTestParser(t)
	merged := p.MergedCollection()
	p.ToggleActiveCollection("Collection B")
	merged.Items[1].Items = nil

	p.SyncMergedCollection(merged)

	if b, _ := p.GetCollection("Collection B"); len(b.Items) != 1 {
		t.Errorf("expected an inactive collection left alone, got %+v", b.Items)
	}
}

// syncTestParser has Dev and Shared active, both defining host, as in
// TestMergedEnvironment_FirstActivatedTakesPrecedence.
func syncTestParser(t *testing.T) *Parser {
	t.Helper()
	p := NewParser()
	for _, environment := range []*Environment{
		{Name: "Dev", Values: []EnvVariable{{Key: "host", Value: "dev", Enabled: true}, {Key: "only_dev", Value: "1", Enabled: true}}},
		{Name: "Shared", Values: []EnvVariable{{Key: "host", Value: "shared", Enabled: true}, {Key: "only_shared", Value: "1", Enabled: true}}},
	} {
		if _, err := p.LoadEnvironment(createTempEnvironment(t, environment)); err != nil {
			t.Fatal(err)
		}
		p.ToggleActiveEnvironment(environment.Name)
	}
	return p
}

func envValue(p *Parser, name, key string) (string, bool) {
	environment, _ := p.GetEnvironment(name)
	for _, variable := range environment.Values {
		if variable.Key == key {
			return variable.Value, true
		}
	}
	return "", false
}

func TestSyncMergedEnvironment_ChangedKeyGoesToTheEnvironmentItResolvesFrom(t *testing.T) {
	p := syncTestParser(t)
	merged := p.MergedEnvironment()
	merged.Values[0].Value = "changed"

	changed := p.SyncMergedEnvironment(merged)

	if dev, _ := envValue(p, "Dev", "host"); dev != "changed" {
		t.Errorf("expected Dev's host changed, got %q", dev)
	}
	if shared, _ := envValue(p, "Shared", "host"); shared != "shared" {
		t.Errorf("expected Shared's shadowed host left alone, got %q", shared)
	}
	if len(changed) != 1 || changed[0] != "Dev" {
		t.Errorf("changed = %v, want [Dev]", changed)
	}
}

func TestSyncMergedEnvironment_NewKeyGoesToTheFirstActiveEnvironment(t *testing.T) {
	p := syncTestParser(t)
	merged := p.MergedEnvironment()
	merged.Values = append(merged.Values, EnvVariable{Key: "token", Value: "t", Enabled: true})

	changed := p.SyncMergedEnvironment(merged)

	if token, ok := envValue(p, "Dev", "token"); !ok || token != "t" {
		t.Errorf("expected the token added to Dev, got %q (defined %v)", token, ok)
	}
	if len(changed) != 1 || changed[0] != "Dev" {
		t.Errorf("changed = %v, want [Dev]", changed)
	}
}

func TestSyncMergedEnvironment_RemovedKeyLeavesEveryActiveEnvironment(t *testing.T) {
	p := syncTestParser(t)
	merged := p.MergedEnvironment()
	merged.Values = slices.DeleteFunc(merged.Values, func(v EnvVariable) bool { return v.Key == "host" })

	changed := p.SyncMergedEnvironment(merged)

	for _, name := range []string{"Dev", "Shared"} {
		if _, ok := envValue(p, name, "host"); ok {
			t.Errorf("expected host removed from %s", name)
		}
	}
	if len(changed) != 2 {
		t.Errorf("changed = %v, want both environments", changed)
	}
}

func TestSyncMergedEnvironment_UnchangedCopyChangesNothing(t *testing.T) {
	p := syncTestParser(t)

	if changed := p.SyncMergedEnvironment(p.MergedEnvironment()); len(changed) != 0 {
		t.Errorf("changed = %v, want none", changed)
	}
}
