package session

import (
	"encoding/json"
	"sync"
)

// State holds per-session accumulated A2UI components.
type State struct {
	SurfaceID     string
	CatalogID     string
	Components    map[string]json.RawMessage // component ID → raw JSON
	DataModel     map[string]any
	Theme         map[string]any
	SendDataModel bool
}

// Store manages session states keyed by MCP session ID.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*State
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]*State)}
}

func (s *Store) GetOrCreate(sessionID string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.sessions[sessionID]; ok {
		return st
	}
	st := &State{
		Components: make(map[string]json.RawMessage),
		DataModel:  make(map[string]any),
	}
	s.sessions[sessionID] = st
	return st
}

func (s *Store) SetSurface(sessionID, surfaceID, catalogID string, theme map[string]any, sendDataModel bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &State{
			Components: make(map[string]json.RawMessage),
			DataModel:  make(map[string]any),
		}
		s.sessions[sessionID] = st
	}
	st.SurfaceID = surfaceID
	st.CatalogID = catalogID
	st.Theme = theme
	st.SendDataModel = sendDataModel
	// Reset components for new surface
	st.Components = make(map[string]json.RawMessage)
	st.DataModel = make(map[string]any)
}

func (s *Store) AddComponent(sessionID string, compJSON json.RawMessage, compID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &State{
			Components: make(map[string]json.RawMessage),
			DataModel:  make(map[string]any),
		}
		s.sessions[sessionID] = st
	}
	st.Components[compID] = compJSON
}

func (s *Store) UpdateDataModel(sessionID string, path string, value map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &State{
			Components: make(map[string]json.RawMessage),
			DataModel:  make(map[string]any),
		}
		s.sessions[sessionID] = st
	}
	if path == "" || path == "/" {
		// Replace entire data model
		for k := range st.DataModel {
			delete(st.DataModel, k)
		}
		for k, v := range value {
			st.DataModel[k] = v
		}
	} else {
		// Store at nested path (e.g., "/data/text_field1/value" → DataModel["data"]["text_field1"]["value"])
		segments := splitPath(path)
		setNestedValue(st.DataModel, segments, value)
	}
}

// splitPath splits "/a/b/c" into ["a", "b", "c"].
func splitPath(path string) []string {
	var parts []string
	for _, p := range splitPathIter(path) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitPathIter(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}

// setNestedValue sets a value at a nested path in a map, creating intermediate maps as needed.
func setNestedValue(m map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}
	for i, key := range keys {
		if i == len(keys)-1 {
			m[key] = value
			return
		}
		next, ok := m[key]
		if !ok {
			next = make(map[string]any)
			m[key] = next
		}
		nextMap, ok := next.(map[string]any)
		if !ok {
			nextMap = make(map[string]any)
			m[key] = nextMap
		}
		m = nextMap
	}
}

func (s *Store) GetState(sessionID string) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[sessionID]
}

func (s *Store) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// SetValue sets a scalar value at a nested path in the data model.
// E.g., SetValue(sessionID, "/data/text_field1/value", "") sets
// DataModel["data"]["text_field1"]["value"] = ""
func (s *Store) SetValue(sessionID string, path string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &State{
			Components: make(map[string]json.RawMessage),
			DataModel:  make(map[string]any),
		}
		s.sessions[sessionID] = st
	}
	segments := splitPath(path)
	setNestedValue(st.DataModel, segments, value)
}
