package postman

import (
	"errors"
	"testing"
)

func scopeCollection() *Collection {
	return &Collection{
		Variables: []Variable{{Key: "base", Value: "c"}},
		Items: []Item{{Name: "Accounts", Variables: []Variable{{Key: "tenant", Value: "a"}}, Items: []Item{
			{Name: "Admin", Items: []Item{{Name: "r", Request: &Request{}}}},
		}}},
	}
}

func TestScopeVariables_OnlyTheScopesOwn(t *testing.T) {
	collection := scopeCollection()

	folder, err := ScopeVariables(collection, []string{"Accounts"})
	if err != nil || len(folder) != 1 || folder[0].Key != "tenant" {
		t.Errorf("expected only the folder's tenant, got %+v (%v)", folder, err)
	}
	nested, _ := ScopeVariables(collection, []string{"Accounts", "Admin"})
	if len(nested) != 0 {
		t.Errorf("expected nothing inherited, got %+v", nested)
	}
	if _, err := ScopeVariables(collection, []string{"Missing"}); !errors.Is(err, ErrScopeNotFound) {
		t.Errorf("expected ErrScopeNotFound, got %v", err)
	}
}

func TestSetScopeVariable_UpdatesOrAddsInThatScope(t *testing.T) {
	collection := scopeCollection()

	if err := SetScopeVariable(collection, []string{"Accounts", "Admin"}, "tenant", VariableValue{Value: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := SetScopeVariable(collection, nil, "base", VariableValue{Value: "c2", Choices: []string{"c", "c2"}, ChoiceNotes: map[string]string{"c2": "new"}}); err != nil {
		t.Fatal(err)
	}

	admin := collection.Items[0].Items[0].Variables
	if len(admin) != 1 || admin[0].Value != "admin" || collection.Items[0].Variables[0].Value != "a" {
		t.Errorf("expected tenant added to Admin only, got admin=%+v accounts=%+v", admin, collection.Items[0].Variables)
	}
	if base := collection.Variables[0]; base.Value != "c2" || base.ChoiceNotes["c2"] != "new" || len(collection.Variables) != 1 {
		t.Errorf("expected base updated in place, got %+v", collection.Variables)
	}
}

func TestDeleteScopeVariable(t *testing.T) {
	collection := scopeCollection()

	if err := DeleteScopeVariable(collection, []string{"Accounts"}, "tenant"); err != nil {
		t.Fatal(err)
	}

	if len(collection.Items[0].Variables) != 0 || len(collection.Variables) != 1 {
		t.Errorf("expected only the folder's tenant removed, got %+v / %+v", collection.Items[0].Variables, collection.Variables)
	}
}
