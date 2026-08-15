package postman

import "strings"

type MergeStrategy int

const (
	MergeKeepOriginal MergeStrategy = iota
	MergeForce
)

// MergeCollections merges src into dst, recursively resolving name collisions
// between folders and requests. dst's Info (name/description/schema) is preserved.
func MergeCollections(dst, src *Collection, strategy MergeStrategy) *Collection {
	return &Collection{
		Info:      dst.Info,
		Items:     mergeItems(dst.Items, src.Items, strategy),
		Variables: mergeVariables(dst.Variables, src.Variables, strategy),
		Events:    mergeEvents(dst.Events, src.Events),
	}
}

// MergeEnvironments merges src into dst by variable key. dst's ID/Name are preserved.
func MergeEnvironments(dst, src *Environment, strategy MergeStrategy) *Environment {
	return &Environment{
		ID:     dst.ID,
		Name:   dst.Name,
		Values: mergeEnvVariables(dst.Values, src.Values, strategy),
	}
}

func mergeItems(dstItems, srcItems []Item, strategy MergeStrategy) []Item {
	result := make([]Item, len(dstItems))
	copy(result, dstItems)

	index := make(map[string]int, len(result))
	for i, item := range result {
		index[item.Name] = i
	}

	for _, srcItem := range srcItems {
		if i, exists := index[srcItem.Name]; exists {
			result[i] = mergeItem(result[i], srcItem, strategy)
			continue
		}
		result = append(result, srcItem)
		index[srcItem.Name] = len(result) - 1
	}

	return result
}

// mergeItem resolves a same-named item pair. Two folders merge recursively;
// anything else (request/request, or a folder/request type mismatch) is
// resolved by strategy, since there is no meaningful way to merge their contents.
func mergeItem(dstItem, srcItem Item, strategy MergeStrategy) Item {
	if dstItem.IsFolder() && srcItem.IsFolder() {
		return Item{
			Name:                    dstItem.Name,
			Items:                   mergeItems(dstItem.Items, srcItem.Items, strategy),
			Description:             dstItem.Description,
			Variables:               mergeVariables(dstItem.Variables, srcItem.Variables, strategy),
			Events:                  mergeEvents(dstItem.Events, srcItem.Events),
			ProtocolProfileBehavior: dstItem.ProtocolProfileBehavior,
		}
	}

	if strategy == MergeForce {
		return srcItem
	}
	return dstItem
}

func mergeVariables(dstVars, srcVars []Variable, strategy MergeStrategy) []Variable {
	result := make([]Variable, len(dstVars))
	copy(result, dstVars)

	index := make(map[string]int, len(result))
	for i, v := range result {
		index[v.Key] = i
	}

	for _, srcVar := range srcVars {
		if i, exists := index[srcVar.Key]; exists {
			if strategy == MergeForce {
				result[i] = srcVar
			}
			continue
		}
		result = append(result, srcVar)
		index[srcVar.Key] = len(result) - 1
	}

	return result
}

func mergeEnvVariables(dstVars, srcVars []EnvVariable, strategy MergeStrategy) []EnvVariable {
	result := make([]EnvVariable, len(dstVars))
	copy(result, dstVars)

	index := make(map[string]int, len(result))
	for i, v := range result {
		index[v.Key] = i
	}

	for _, srcVar := range srcVars {
		if i, exists := index[srcVar.Key]; exists {
			if strategy == MergeForce {
				result[i] = srcVar
			}
			continue
		}
		result = append(result, srcVar)
		index[srcVar.Key] = len(result) - 1
	}

	return result
}

func mergeEvents(dstEvents, srcEvents []Event) []Event {
	result := make([]Event, len(dstEvents))
	copy(result, dstEvents)

	seen := make(map[string]bool, len(result))
	for _, e := range result {
		seen[eventKey(e)] = true
	}

	for _, e := range srcEvents {
		key := eventKey(e)
		if seen[key] {
			continue
		}
		result = append(result, e)
		seen[key] = true
	}

	return result
}

func eventKey(e Event) string {
	return e.Listen + "|" + strings.Join(e.Script.Exec, "\n")
}
