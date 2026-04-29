package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/juncaifeng/a2ui-mcp-server/internal/catalog"
)

// GenerateToolSchema creates a simplified MCP tool inputSchema for a component.
func GenerateToolSchema(comp *catalog.ComponentDef) (json.RawMessage, error) {
	props := make(map[string]any)
	required := make([]string, 0)

	for _, propName := range sortedKeys(comp.Properties) {
		pd := comp.Properties[propName]

		// Skip const fields (component name is set automatically)
		if pd.IsConst {
			continue
		}

		// Map property to MCP tool input
		switch {
		case propName == "id":
			props["id"] = map[string]any{
				"type":        "string",
				"description": "Unique component ID within the surface",
			}
			required = append(required, "id")

		case pd.IsDynamicString:
			addDynamicStringFields(props, propName, pd, comp)
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.IsDynamicNumber:
			addDynamicNumberFields(props, propName, pd, comp)

		case pd.IsDynamicBoolean:
			addDynamicBooleanFields(props, propName, pd, comp)

		case pd.IsChildList:
			props["children"] = map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": pd.Description,
			}
			required = append(required, "children")

		case pd.IsComponentId:
			desc := pd.Description
			if desc == "" {
				desc = fmt.Sprintf("ID of the child component")
			}
			props[propName] = map[string]any{
				"type":        "string",
				"description": desc,
			}
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.IsAction:
			props["action_event"] = map[string]any{
				"type":        "string",
				"description": "Name of the action event to dispatch to the server",
			}
			props["action_context"] = map[string]any{
				"type":        "object",
				"description": "Optional context key-value pairs for the action event",
				"additionalProperties": map[string]any{},
			}
			if comp.IsRequired(propName) {
				required = append(required, "action_event")
			}

		case pd.Type == "string" && len(pd.Enum) > 0:
			prop := map[string]any{
				"type":        "string",
				"description": pd.Description,
				"enum":        pd.Enum,
			}
			if pd.Default != nil {
				prop["default"] = pd.Default
			}
			props[propName] = prop
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.Type == "string":
			props[propName] = map[string]any{
				"type":        "string",
				"description": pd.Description,
			}
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.Type == "number":
			props[propName] = map[string]any{
				"type":        "number",
				"description": pd.Description,
			}
			if pd.Default != nil {
				props[propName].(map[string]any)["default"] = pd.Default
			}
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.Type == "boolean":
			props[propName] = map[string]any{
				"type":        "boolean",
				"description": pd.Description,
			}
			if pd.Default != nil {
				props[propName].(map[string]any)["default"] = pd.Default
			}

		case pd.Type == "array" && pd.ItemsProperties != nil:
			// Complex array items (e.g., Tabs.tabs, ChoicePicker.options)
			itemProps := make(map[string]any)
			itemRequired := make([]string, 0)
			for k, v := range pd.ItemsProperties {
				p := map[string]any{"type": mapType(v.Type)}
				if v.Description != "" {
					p["description"] = v.Description
				}
				if v.IsDynamicString {
					p["type"] = "string"
				}
				itemProps[k] = p
				for _, r := range pd.ItemsRequired {
					if r == k {
						itemRequired = append(itemRequired, k)
					}
				}
			}
			props[propName] = map[string]any{
				"type":        "array",
				"description": pd.Description,
				"items": map[string]any{
					"type":                 "object",
					"properties":           itemProps,
					"required":             itemRequired,
					"additionalProperties": false,
				},
			}
			if comp.IsRequired(propName) {
				required = append(required, propName)
			}

		case pd.Type == "object":
			props[propName] = map[string]any{
				"type":        "object",
				"description": pd.Description,
			}
		}

		// Add weight as a common optional field (from CatalogComponentCommon)
		if propName == "weight" {
			// Already handled as number above
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}

	return json.Marshal(schema)
}

func addDynamicStringFields(props map[string]any, name string, pd catalog.PropertyDef, comp *catalog.ComponentDef) {
	desc := pd.Description
	if desc == "" {
		desc = fmt.Sprintf("The %s value", name)
	}
	props[name] = map[string]any{
		"type":        "string",
		"description": desc,
	}
	props[name+"_path"] = map[string]any{
		"type":        "string",
		"description": fmt.Sprintf("JSON Pointer path for data-bound %s (use instead of literal value)", name),
	}
}

func addDynamicNumberFields(props map[string]any, name string, pd catalog.PropertyDef, comp *catalog.ComponentDef) {
	desc := pd.Description
	if desc == "" {
		desc = fmt.Sprintf("The %s value", name)
	}
	props[name] = map[string]any{
		"type":        "number",
		"description": desc,
	}
	props[name+"_path"] = map[string]any{
		"type":        "string",
		"description": fmt.Sprintf("JSON Pointer path for data-bound %s", name),
	}
}

func addDynamicBooleanFields(props map[string]any, name string, pd catalog.PropertyDef, comp *catalog.ComponentDef) {
	desc := pd.Description
	if desc == "" {
		desc = fmt.Sprintf("The %s value", name)
	}
	props[name] = map[string]any{
		"type":        "boolean",
		"description": desc,
	}
	props[name+"_path"] = map[string]any{
		"type":        "string",
		"description": fmt.Sprintf("JSON Pointer path for data-bound %s", name),
	}
}

func mapType(t string) string {
	return t
}

func sortedKeys(m map[string]catalog.PropertyDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort for deterministic output
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// ToolName converts a component name to a tool name: "Text" → "create_text"
func ToolName(compName string) string {
	return "create_" + toSnakeCase(compName)
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ToolDescription generates a description for a component tool.
func ToolDescription(comp *catalog.ComponentDef) string {
	desc := comp.Desc
	if desc == "" {
		desc = fmt.Sprintf("Create a %s component", comp.Name)
	}
	return fmt.Sprintf("Create an A2UI %s component. %s", comp.Name, desc)
}
