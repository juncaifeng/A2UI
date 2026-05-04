package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juncaifeng/a2ui-mcp-server/internal/a2ui"
	"github.com/juncaifeng/a2ui-mcp-server/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RenderUIInput struct {
	SurfaceID string `json:"surface_id,omitempty" jsonschema:"The surface to render. If omitted, renders all surfaces."`
}

// RegisterRenderTools registers the render_ui tool.
func RegisterRenderTools(server *mcp.Server, store *session.Store, builder *a2ui.Builder) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "render_ui",
		Title:       "Render UI",
		Description: "Assemble all accumulated components into a complete A2UI JSON response. Call this after creating a surface and adding components.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenderUIInput) (*mcp.CallToolResult, any, error) {
		sessionID := getSessionID(req)

		var surfaces []*session.SurfaceState
		if args.SurfaceID != "" {
			sf := store.GetSurface(sessionID, args.SurfaceID)
			if sf == nil {
				return errorResult(fmt.Sprintf("Surface %q not found. Call create_surface first.", args.SurfaceID)), nil, nil
			}
			surfaces = []*session.SurfaceState{sf}
		} else {
			surfaces = store.GetAllSurfaces(sessionID)
			if len(surfaces) == 0 {
				return errorResult("No surfaces found. Call create_surface first."), nil, nil
			}
		}

		var allMessages []json.RawMessage
		totalComps := 0

		for _, sf := range surfaces {
			// Validate component tree
			errs := a2ui.ValidateTree(sf.Components)
			if len(errs) > 0 {
				var msgs []string
				for _, e := range errs {
					msgs = append(msgs, e.Error())
				}
				return errorResult(fmt.Sprintf("Validation failed on surface %q:\n- %s", sf.SurfaceID, strings.Join(msgs, "\n- "))), nil, nil
			}

			messages, err := builder.BuildMessages(sf)
			if err != nil {
				return errorResult(fmt.Sprintf("Build failed for surface %q: %v", sf.SurfaceID, err)), nil, nil
			}

			allMessages = append(allMessages, messages...)
			totalComps += len(sf.Components)
		}

		messagesJSON, err := json.MarshalIndent(allMessages, "", "  ")
		if err != nil {
			return errorResult(fmt.Sprintf("JSON marshal failed: %v", err)), nil, nil
		}

		// Determine render URI
		renderURI := "a2ui://render/all"
		if args.SurfaceID != "" {
			renderURI = fmt.Sprintf("a2ui://render/%s", args.SurfaceID)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Rendered %d components across %d surface(s)", totalComps, len(surfaces))},
				&mcp.EmbeddedResource{
					Resource: &mcp.ResourceContents{
						URI:      renderURI,
						MIMEType: "application/json+a2ui",
						Text:     string(messagesJSON),
					},
				},
			},
		}, nil, nil
	})
}
