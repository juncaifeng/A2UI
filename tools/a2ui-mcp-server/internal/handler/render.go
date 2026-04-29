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
	SurfaceID string `json:"surface_id" jsonschema:"The surface to render"`
}

// RegisterRenderTools registers the render_ui tool.
func RegisterRenderTools(server *mcp.Server, store *session.Store, builder *a2ui.Builder) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "render_ui",
		Title:       "Render UI",
		Description: "Assemble all accumulated components into a complete A2UI JSON response. Call this after creating a surface and adding components.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenderUIInput) (*mcp.CallToolResult, any, error) {
		sessionID := getSessionID(req)
		st := store.GetState(sessionID)
		if st == nil {
			return errorResult("No session found. Call create_surface first."), nil, nil
		}

		// Validate component tree
		errs := a2ui.ValidateTree(st.Components)
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			return errorResult(fmt.Sprintf("Validation failed:\n- %s", strings.Join(msgs, "\n- "))), nil, nil
		}

		// Build complete A2UI messages
		messages, err := builder.BuildMessages(st)
		if err != nil {
			return errorResult(fmt.Sprintf("Build failed: %v", err)), nil, nil
		}

		messagesJSON, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return errorResult(fmt.Sprintf("JSON marshal failed: %v", err)), nil, nil
		}

		compCount := len(st.Components)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Rendered %d components on surface %q", compCount, st.SurfaceID)},
				&mcp.EmbeddedResource{
					Resource: &mcp.ResourceContents{
						URI:      fmt.Sprintf("a2ui://render/%s", st.SurfaceID),
						MIMEType: "application/json+a2ui",
						Text:     string(messagesJSON),
					},
				},
			},
		}, nil, nil
	})
}
