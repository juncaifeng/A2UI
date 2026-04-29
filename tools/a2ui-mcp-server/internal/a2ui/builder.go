package a2ui

import (
	"encoding/json"
	"fmt"

	"github.com/juncaifeng/a2ui-mcp-server/internal/session"
)

// Builder assembles complete A2UI message arrays from session state.
type Builder struct{}

func NewBuilder() *Builder {
	return &Builder{}
}

// BuildMessages creates the full A2UI message array for a session.
func (b *Builder) BuildMessages(st *session.State) ([]json.RawMessage, error) {
	var messages []json.RawMessage

	// 1. createSurface
	if st.SurfaceID == "" {
		return nil, fmt.Errorf("no surface created; call create_surface first")
	}
	catalogID := st.CatalogID
	if catalogID == "" {
		catalogID = "https://a2ui.org/specification/v0_9/basic_catalog.json"
	}

	createMsg := CreateSurfaceMessage{
		Version: Version,
		CreateSurface: CreateSurface{
			SurfaceID:     st.SurfaceID,
			CatalogID:     catalogID,
			Theme:         st.Theme,
			SendDataModel: st.SendDataModel,
		},
	}
	raw, _ := json.Marshal(createMsg)
	messages = append(messages, raw)

	// 2. updateComponents
	if len(st.Components) > 0 {
		components := make([]map[string]any, 0, len(st.Components))
		for _, compJSON := range st.Components {
			var comp map[string]any
			_ = json.Unmarshal(compJSON, &comp)
			components = append(components, comp)
		}
		updateMsg := UpdateComponentsMessage{
			Version: Version,
			UpdateComponents: UpdateComponents{
				SurfaceID:  st.SurfaceID,
				Components: components,
			},
		}
		raw, _ = json.Marshal(updateMsg)
		messages = append(messages, raw)
	}

	// 3. updateDataModel
	if len(st.DataModel) > 0 {
		dmMsg := UpdateDataModelMessage{
			Version: Version,
			UpdateDataModel: UpdateDataModel{
				SurfaceID: st.SurfaceID,
				Path:      "/",
				Value:     st.DataModel,
			},
		}
		raw, _ = json.Marshal(dmMsg)
		messages = append(messages, raw)
	}

	return messages, nil
}

// BuildCreateSurface creates just the createSurface message.
func (b *Builder) BuildCreateSurface(st *session.State) (json.RawMessage, error) {
	catalogID := st.CatalogID
	if catalogID == "" {
		catalogID = "https://a2ui.org/specification/v0_9/basic_catalog.json"
	}
	msg := CreateSurfaceMessage{
		Version: Version,
		CreateSurface: CreateSurface{
			SurfaceID:     st.SurfaceID,
			CatalogID:     catalogID,
			Theme:         st.Theme,
			SendDataModel: st.SendDataModel,
		},
	}
	return json.Marshal(msg)
}

// BuildUpdateDataModel creates an updateDataModel message.
func (b *Builder) BuildUpdateDataModel(surfaceID, path string, value map[string]any) (json.RawMessage, error) {
	msg := UpdateDataModelMessage{
		Version: Version,
		UpdateDataModel: UpdateDataModel{
			SurfaceID: surfaceID,
			Path:      path,
			Value:     value,
		},
	}
	return json.Marshal(msg)
}

// BuildDeleteSurface creates a deleteSurface message.
func (b *Builder) BuildDeleteSurface(surfaceID string) (json.RawMessage, error) {
	msg := DeleteSurfaceMessage{Version: Version}
	msg.DeleteSurface.SurfaceID = surfaceID
	return json.Marshal(msg)
}
