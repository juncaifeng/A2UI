package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/juncaifeng/a2ui-mcp-server/internal/catalog"
	"github.com/juncaifeng/a2ui-mcp-server/internal/schema"
	"github.com/juncaifeng/a2ui-mcp-server/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterComponentTools registers all component tools from the catalog.
func RegisterComponentTools(server *mcp.Server, cat *catalog.Catalog, store *session.Store) error {
	for compName, compDef := range cat.Components {
		toolName := schema.ToolName(compName)
		toolDesc := schema.ToolDescription(compDef)

		inputSchema, err := schema.GenerateToolSchema(compDef)
		if err != nil {
			return fmt.Errorf("generate schema for %s: %w", compName, err)
		}

		// Capture loop variable
		name := compName
		def := compDef

		server.AddTool(&mcp.Tool{
			Name:        toolName,
			Title:       fmt.Sprintf("Create %s", compName),
			Description: toolDesc,
			InputSchema: inputSchema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleComponentCreate(req, store, name, def)
		})

		log.Printf("Registered tool: %s", toolName)
	}
	return nil
}

func handleComponentCreate(req *mcp.CallToolRequest, store *session.Store, compName string, compDef *catalog.ComponentDef) (*mcp.CallToolResult, error) {
	log.Printf("[DEBUG] component handler ENTERED: comp=%s", compName)
	sessionID := req.Session.ID()
	if sessionID == "" {
		sessionID = "default"
	}

	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return errorResult(fmt.Sprintf("Failed to parse arguments: %v", err)), nil
	}

	// Get component ID
	id, _ := args["id"].(string)
	if id == "" {
		return errorResult("Missing required field: id"), nil
	}

	// Build the A2UI component JSON
	comp := buildA2UIComponent(compName, compDef, args)

	// Auto-generate data model bindings for DynamicString/DynamicNumber/DynamicBoolean
	// properties that don't already have a path binding.
	// This is required for interactive components (TextField, CheckBox, Slider, etc.)
	// whose setValue() only works when the property has a {path: "..."} binding.
	for propName, pd := range compDef.Properties {
		if pd.IsConst || propName == "id" {
			continue
		}
		if !pd.IsDynamicString && !pd.IsDynamicNumber && !pd.IsDynamicBoolean && !pd.IsDynamicStringList {
			continue
		}
		// Only auto-bind if no path was already set
		rawVal, exists := comp[propName]
		if exists {
			if m, ok := rawVal.(map[string]any); ok {
				if _, hasPath := m["path"]; hasPath {
					continue
				}
			}
		}
		// Get initial value (use zero value if not provided)
		var initVal any = rawVal
		if !exists {
			switch {
			case pd.IsDynamicString:
				initVal = ""
			case pd.IsDynamicNumber:
				initVal = float64(0)
			case pd.IsDynamicBoolean:
				initVal = false
			case pd.IsDynamicStringList:
				initVal = []any{}
			}
		}
		// Replace with path binding
		dataPath := fmt.Sprintf("/data/%s/%s", id, propName)
		comp[propName] = map[string]any{"path": dataPath}
		// Register initial value in data model
		store.SetValue(sessionID, dataPath, initVal)
	}

	compJSON, err := json.Marshal(comp)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to marshal component: %v", err)), nil
	}

	// Store in session
	store.AddComponent(sessionID, compJSON, id)

	result := fmt.Sprintf("Created %s component with id=%q", compName, id)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
			&mcp.EmbeddedResource{
				Resource: &mcp.ResourceContents{
					URI:      fmt.Sprintf("a2ui://component/%s", id),
					MIMEType: "application/json+a2ui",
					Text:     string(compJSON),
				},
			},
		},
	}, nil
}

func buildA2UIComponent(compName string, compDef *catalog.ComponentDef, args map[string]any) map[string]any {
	comp := map[string]any{
		"id":        args["id"],
		"component": compName,
	}

	for propName, pd := range compDef.Properties {
		if pd.IsConst || propName == "id" {
			continue
		}

		switch {
		case pd.IsDynamicString:
			val, hasVal := args[propName].(string)
			pathVal, hasPath := args[propName+"_path"].(string)
			if hasPath && pathVal != "" {
				comp[propName] = map[string]any{"path": pathVal}
			} else if hasVal && val != "" {
				comp[propName] = val
			}

		case pd.IsDynamicNumber:
			if val, ok := toFloat(args[propName]); ok {
				comp[propName] = val
			} else if pathVal, ok := args[propName+"_path"].(string); ok && pathVal != "" {
				comp[propName] = map[string]any{"path": pathVal}
			}

		case pd.IsDynamicBoolean:
			if val, ok := args[propName].(bool); ok {
				comp[propName] = val
			} else if pathVal, ok := args[propName+"_path"].(string); ok && pathVal != "" {
				comp[propName] = map[string]any{"path": pathVal}
			}

		case pd.IsChildList:
			if children, ok := args[propName].([]any); ok && len(children) > 0 {
				// Convert to string array
				strChildren := make([]string, 0, len(children))
				for _, c := range children {
					if s, ok := c.(string); ok {
						strChildren = append(strChildren, s)
					}
				}
				comp[propName] = strChildren
			}

		case pd.IsDynamicStringList:
			if val, ok := args[propName].([]any); ok {
				comp[propName] = val
			} else if pathVal, ok := args[propName+"_path"].(string); ok && pathVal != "" {
				comp[propName] = map[string]any{"path": pathVal}
			}

		case pd.IsComponentId:
			if val, ok := args[propName].(string); ok && val != "" {
				comp[propName] = val
			}

		case pd.IsAction:
			eventName, _ := args["action_event"].(string)
			if eventName != "" {
				action := map[string]any{
					"event": map[string]any{
						"name": eventName,
					},
				}
				if ctx, ok := args["action_context"].(map[string]any); ok && len(ctx) > 0 {
					action["event"].(map[string]any)["context"] = ctx
				}
				comp["action"] = action
			}

		case pd.Type == "array" && pd.ItemsProperties != nil:
			if arr, ok := args[propName].([]any); ok && len(arr) > 0 {
				items := make([]map[string]any, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]any); ok {
						entry := make(map[string]any)
						for itemPropName, itemPropDef := range pd.ItemsProperties {
							if v, exists := m[itemPropName]; exists {
								if itemPropDef.IsDynamicString {
									entry[itemPropName] = v // plain string is valid DynamicString
								} else {
									entry[itemPropName] = v
								}
							}
						}
						items = append(items, entry)
					}
				}
				comp[propName] = items
			}

		case pd.Type == "string" && len(pd.Enum) > 0:
			if val, ok := args[propName].(string); ok && val != "" {
				comp[propName] = val
			} else if pd.Default != nil {
				comp[propName] = pd.Default
			}

		case pd.Type == "string":
			if val, ok := args[propName].(string); ok && val != "" {
				comp[propName] = val
			}

		case pd.Type == "number":
			if val, ok := toFloat(args[propName]); ok {
				comp[propName] = val
			} else if pd.Default != nil {
				comp[propName] = pd.Default
			}

		case pd.Type == "boolean":
			if val, ok := args[propName].(bool); ok {
				comp[propName] = val
			} else if pd.Default != nil {
				comp[propName] = pd.Default
			}

		case pd.Type == "object":
			if val, ok := args[propName].(map[string]any); ok {
				comp[propName] = val
			}
		}
	}

	return comp
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

