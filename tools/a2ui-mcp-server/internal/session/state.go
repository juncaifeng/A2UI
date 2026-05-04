package session

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// ErrMaxSurfaces is returned when a session exceeds the maximum number of surfaces.
var ErrMaxSurfaces = errors.New("maximum number of surfaces per session reached")

const (
	MaxSurfacesPerSession = 5
	SessionTTL            = 30 * time.Minute
	CleanupInterval       = 5 * time.Minute
)

// SurfaceState holds the accumulated A2UI components for a single surface.
type SurfaceState struct {
	SurfaceID     string
	CatalogID     string
	Components    map[string]json.RawMessage // component ID → raw JSON
	DataModel     map[string]any
	Theme         map[string]any
	SendDataModel bool
}

// State holds per-session data: one or more surfaces + session metadata.
type State struct {
	Surfaces       map[string]*SurfaceState
	DefaultSurface string
	LastAccess     time.Time
}

// Store manages session states keyed by MCP session ID.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*State
	stopCh   chan struct{}
}

func NewStore() *Store {
	s := &Store{
		sessions: make(map[string]*State),
		stopCh:   make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// Stop terminates the background cleanup goroutine.
func (s *Store) Stop() {
	close(s.stopCh)
}

func (s *Store) getOrCreateSession(sessionID string) *State {
	st, ok := s.sessions[sessionID]
	if !ok {
		st = &State{
			Surfaces:   make(map[string]*SurfaceState),
			LastAccess: time.Now(),
		}
		s.sessions[sessionID] = st
	}
	st.LastAccess = time.Now()
	return st
}

// GetOrCreate returns the default surface for a session, creating session if needed.
func (s *Store) GetOrCreate(sessionID string) *SurfaceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreateSession(sessionID)
	if st.DefaultSurface == "" {
		return nil
	}
	return st.Surfaces[st.DefaultSurface]
}

// GetSurface returns a specific surface, or nil if not found.
func (s *Store) GetSurface(sessionID, surfaceID string) *SurfaceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		return nil
	}
	st.LastAccess = time.Now()
	return st.Surfaces[surfaceID]
}

// GetAllSurfaces returns all surfaces for a session.
func (s *Store) GetAllSurfaces(sessionID string) []*SurfaceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		return nil
	}
	st.LastAccess = time.Now()
	result := make([]*SurfaceState, 0, len(st.Surfaces))
	for _, sf := range st.Surfaces {
		result = append(result, sf)
	}
	return result
}

// SetSurface creates or resets a surface. Wipes all components and data model (idempotent).
// Returns error if max surfaces limit is reached (unless surface already exists).
func (s *Store) SetSurface(sessionID, surfaceID, catalogID string, theme map[string]any, sendDataModel bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreateSession(sessionID)

	if _, exists := st.Surfaces[surfaceID]; !exists {
		if len(st.Surfaces) >= MaxSurfacesPerSession {
			return ErrMaxSurfaces
		}
	}

	st.Surfaces[surfaceID] = &SurfaceState{
		SurfaceID:     surfaceID,
		CatalogID:     catalogID,
		Components:    make(map[string]json.RawMessage),
		DataModel:     make(map[string]any),
		Theme:         theme,
		SendDataModel: sendDataModel,
	}
	st.DefaultSurface = surfaceID
	return nil
}

// DeleteSurface removes a specific surface from a session.
func (s *Store) DeleteSurface(sessionID, surfaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	st.LastAccess = time.Now()
	delete(st.Surfaces, surfaceID)
	if st.DefaultSurface == surfaceID {
		st.DefaultSurface = ""
		// Promote another surface as default
		for id := range st.Surfaces {
			st.DefaultSurface = id
			break
		}
	}
	// Clean up empty sessions
	if len(st.Surfaces) == 0 {
		delete(s.sessions, sessionID)
	}
}

// AddComponent adds a component to the default surface of a session.
func (s *Store) AddComponent(sessionID string, compJSON json.RawMessage, compID string) {
	s.AddComponentTo(sessionID, "", compJSON, compID)
}

// AddComponentTo adds a component to a specific surface (empty surfaceID = default).
func (s *Store) AddComponentTo(sessionID, surfaceID string, compJSON json.RawMessage, compID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sf := s.getSurfaceLocked(sessionID, surfaceID)
	if sf == nil {
		return
	}
	sf.Components[compID] = compJSON
}

// UpdateDataModel updates the data model on the default surface.
func (s *Store) UpdateDataModel(sessionID string, path string, value map[string]any) {
	s.UpdateDataModelOn(sessionID, "", path, value)
}

// UpdateDataModelOn updates the data model on a specific surface (empty surfaceID = default).
func (s *Store) UpdateDataModelOn(sessionID, surfaceID string, path string, value map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sf := s.getSurfaceLocked(sessionID, surfaceID)
	if sf == nil {
		return
	}
	if path == "" || path == "/" {
		for k := range sf.DataModel {
			delete(sf.DataModel, k)
		}
		for k, v := range value {
			sf.DataModel[k] = v
		}
	} else {
		segments := splitPath(path)
		setNestedValue(sf.DataModel, segments, value)
	}
}

// SetValue sets a scalar value at a nested path on the default surface.
func (s *Store) SetValue(sessionID string, path string, value any) {
	s.SetValueOn(sessionID, "", path, value)
}

// SetValueOn sets a scalar value at a nested path on a specific surface (empty surfaceID = default).
func (s *Store) SetValueOn(sessionID, surfaceID string, path string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sf := s.getSurfaceLocked(sessionID, surfaceID)
	if sf == nil {
		return
	}
	segments := splitPath(path)
	setNestedValue(sf.DataModel, segments, value)
}

// Clear removes an entire session and all its surfaces.
func (s *Store) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// getSurfaceLocked returns the surface state (must be called with mu held).
func (s *Store) getSurfaceLocked(sessionID, surfaceID string) *SurfaceState {
	st := s.getOrCreateSession(sessionID)
	if surfaceID != "" {
		return st.Surfaces[surfaceID]
	}
	if st.DefaultSurface != "" {
		return st.Surfaces[st.DefaultSurface]
	}
	return nil
}

// cleanupLoop periodically removes expired sessions.
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.evictExpired()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Store) evictExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, st := range s.sessions {
		if now.Sub(st.LastAccess) > SessionTTL {
			delete(s.sessions, id)
		}
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
