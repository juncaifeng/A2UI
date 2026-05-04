package handler

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/juncaifeng/a2ui-mcp-server/internal/a2ui"
	"github.com/juncaifeng/a2ui-mcp-server/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Protocol Tool Input Types ---

type CreateSurfaceInput struct {
	SurfaceID     string         `json:"surface_id" jsonschema:"Unique identifier for the UI surface"`
	CatalogID     string         `json:"catalog_id,omitempty" jsonschema:"Catalog ID. Defaults to loaded catalog."`
	Theme         map[string]any `json:"theme,omitempty" jsonschema:"Theme config: primaryColor, iconUrl, agentDisplayName"`
	SendDataModel bool           `json:"send_data_model,omitempty" jsonschema:"If true, client sends data model with every action"`
}

type UpdateDataModelInput struct {
	SurfaceID string         `json:"surface_id" jsonschema:"Surface to update"`
	Path      string         `json:"path,omitempty" jsonschema:"JSON Pointer path. Defaults to root."`
	Value     map[string]any `json:"value" jsonschema:"The data to set at the given path"`
}

type DeleteSurfaceInput struct {
	SurfaceID string `json:"surface_id" jsonschema:"Surface to delete"`
}

var surfaceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func validateSurfaceID(id string) error {
	if id == "" {
		return fmt.Errorf("surface_id must not be empty")
	}
	if len(id) > 64 {
		return fmt.Errorf("surface_id must be at most 64 characters")
	}
	if !surfaceIDPattern.MatchString(id) {
		return fmt.Errorf("surface_id must start with a letter or digit and contain only [a-zA-Z0-9_-]")
	}
	return nil
}

// RegisterProtocolTools registers the 3 protocol tools (create_surface, update_data_model, delete_surface).
func RegisterProtocolTools(server *mcp.Server, store *session.Store, builder *a2ui.Builder, defaultCatalogID string) {
	// create_surface
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_surface",
		Title:       "Create Surface",
		Description: "Create a new A2UI surface (UI canvas). Must be called before adding components.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateSurfaceInput) (*mcp.CallToolResult, any, error) {
		log.Printf("[DEBUG] create_surface HANDLER ENTERED: surface_id=%q", args.SurfaceID)
		sessionID := getSessionID(req)
		log.Printf("[DEBUG] create_surface sessionID=%q", sessionID)

		if err := validateSurfaceID(args.SurfaceID); err != nil {
			return errorResult(err.Error()), nil, nil
		}

		catalogID := args.CatalogID
		if catalogID == "" {
			catalogID = defaultCatalogID
		}

		if err := store.SetSurface(sessionID, args.SurfaceID, catalogID, args.Theme, args.SendDataModel); err != nil {
			return errorResult(fmt.Sprintf("Cannot create surface: %v", err)), nil, nil
		}

		sf := store.GetSurface(sessionID, args.SurfaceID)
		msg, _ := builder.BuildCreateSurface(sf)
		log.Printf("[DEBUG] create_surface HANDLER RETURNING OK: surface_id=%q", args.SurfaceID)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Surface %q created", args.SurfaceID)},
				&mcp.EmbeddedResource{
					Resource: &mcp.ResourceContents{
						URI:      fmt.Sprintf("a2ui://surface/%s", args.SurfaceID),
						MIMEType: "application/json+a2ui",
						Text:     string(msg),
					},
				},
			},
		}, nil, nil
	})

	// update_data_model
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_data_model",
		Title:       "Update Data Model",
		Description: "Update the data model for a surface. Components bound to data paths will reactively update.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateDataModelInput) (*mcp.CallToolResult, any, error) {
		sessionID := getSessionID(req)
		path := args.Path
		if path == "" {
			path = "/"
		}
		store.UpdateDataModelOn(sessionID, args.SurfaceID, path, args.Value)

		msg, _ := builder.BuildUpdateDataModel(args.SurfaceID, path, args.Value)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Data model updated at path %q", path)},
				&mcp.EmbeddedResource{
					Resource: &mcp.ResourceContents{
						URI:      fmt.Sprintf("a2ui://datamodel/%s", args.SurfaceID),
						MIMEType: "application/json+a2ui",
						Text:     string(msg),
					},
				},
			},
		}, nil, nil
	})

	// delete_surface
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_surface",
		Title:       "Delete Surface",
		Description: "Delete an A2UI surface and all its components.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args DeleteSurfaceInput) (*mcp.CallToolResult, any, error) {
		sessionID := getSessionID(req)

		msg, _ := builder.BuildDeleteSurface(args.SurfaceID)
		store.DeleteSurface(sessionID, args.SurfaceID)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Surface %q deleted", args.SurfaceID)},
				&mcp.EmbeddedResource{
					Resource: &mcp.ResourceContents{
						URI:      fmt.Sprintf("a2ui://delete/%s", args.SurfaceID),
						MIMEType: "application/json+a2ui",
						Text:     string(msg),
					},
				},
			},
		}, nil, nil
	})
}

func getSessionID(req *mcp.CallToolRequest) string {
	if req.Session != nil {
		return req.Session.ID()
	}
	return "default"
}
