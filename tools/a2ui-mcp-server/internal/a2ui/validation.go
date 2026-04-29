package a2ui

import (
	"encoding/json"
	"fmt"
)

// ValidationError represents a problem found in the component tree.
type ValidationError struct {
	ComponentID string
	Message     string
}

func (e ValidationError) Error() string {
	if e.ComponentID != "" {
		return fmt.Sprintf("component %q: %s", e.ComponentID, e.Message)
	}
	return e.Message
}

// ValidateTree checks the component tree for structural integrity.
func ValidateTree(components map[string]json.RawMessage) []ValidationError {
	var errs []ValidationError

	if len(components) == 0 {
		return errs
	}

	// 1. Check root exists
	if _, ok := components["root"]; !ok {
		errs = append(errs, ValidationError{Message: "no component with id 'root' found"})
	}

	// Build lookup set
	ids := make(map[string]bool, len(components))
	for id := range components {
		ids[id] = true
	}

	// 2. Check all child references resolve
	for id, raw := range components {
		var comp map[string]any
		if err := json.Unmarshal(raw, &comp); err != nil {
			errs = append(errs, ValidationError{ComponentID: id, Message: "invalid JSON"})
			continue
		}

		// Check "child" field (single ComponentId)
		if child, ok := comp["child"].(string); ok && child != "" {
			if !ids[child] {
				errs = append(errs, ValidationError{
					ComponentID: id,
					Message:     fmt.Sprintf("references non-existent child %q", child),
				})
			}
		}

		// Check "children" field (ChildList array)
		if children, ok := comp["children"].([]any); ok {
			for _, c := range children {
				if childID, ok := c.(string); ok && childID != "" {
					if !ids[childID] {
						errs = append(errs, ValidationError{
							ComponentID: id,
							Message:     fmt.Sprintf("references non-existent child %q", childID),
						})
					}
				}
			}
		}

		// Check tabs[].child
		if tabs, ok := comp["tabs"].([]any); ok {
			for _, t := range tabs {
				if tab, ok := t.(map[string]any); ok {
					if childID, ok := tab["child"].(string); ok && childID != "" {
						if !ids[childID] {
							errs = append(errs, ValidationError{
								ComponentID: id,
								Message:     fmt.Sprintf("tab references non-existent child %q", childID),
							})
						}
					}
				}
			}
		}

		// Check modal trigger and content
		for _, field := range []string{"trigger", "content"} {
			if ref, ok := comp[field].(string); ok && ref != "" {
				if !ids[ref] {
					errs = append(errs, ValidationError{
						ComponentID: id,
						Message:     fmt.Sprintf("%s references non-existent component %q", field, ref),
					})
				}
			}
		}
	}

	// 3. Check for circular references
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	for id := range components {
		if hasCycle(id, components, visited, inStack) {
			errs = append(errs, ValidationError{Message: "circular reference detected in component tree"})
			break
		}
	}

	return errs
}

func hasCycle(id string, components map[string]json.RawMessage, visited, inStack map[string]bool) bool {
	if inStack[id] {
		return true
	}
	if visited[id] {
		return false
	}
	visited[id] = true
	inStack[id] = true
	defer func() { inStack[id] = false }()

	raw, ok := components[id]
	if !ok {
		return false
	}
	var comp map[string]any
	if err := json.Unmarshal(raw, &comp); err != nil {
		return false
	}

	// Collect all referenced child IDs
	var childIDs []string
	if child, ok := comp["child"].(string); ok {
		childIDs = append(childIDs, child)
	}
	if children, ok := comp["children"].([]any); ok {
		for _, c := range children {
			if s, ok := c.(string); ok {
				childIDs = append(childIDs, s)
			}
		}
	}

	for _, cid := range childIDs {
		if hasCycle(cid, components, visited, inStack) {
			return true
		}
	}
	return false
}
