package a2ui

import (
	"encoding/json"
)

const Version = "v0.9"

// DynamicString marshals as either a plain string or {"path": "..."} or a function call.
type DynamicString struct {
	literal string
	path    string
	fnCall  *FunctionCall
}

func LiteralString(s string) DynamicString {
	return DynamicString{literal: s}
}

func BoundString(path string) DynamicString {
	return DynamicString{path: path}
}

func FunctionString(fc *FunctionCall) DynamicString {
	return DynamicString{fnCall: fc}
}

func (d DynamicString) MarshalJSON() ([]byte, error) {
	if d.path != "" {
		return json.Marshal(&DataBinding{Path: d.path})
	}
	if d.fnCall != nil {
		return json.Marshal(d.fnCall)
	}
	return json.Marshal(d.literal)
}

type DynamicNumber struct {
	literal float64
	set     bool
	path    string
	fnCall  *FunctionCall
}

func LiteralNumber(n float64) DynamicNumber {
	return DynamicNumber{literal: n, set: true}
}

func BoundNumber(path string) DynamicNumber {
	return DynamicNumber{path: path}
}

func (d DynamicNumber) MarshalJSON() ([]byte, error) {
	if d.path != "" {
		return json.Marshal(&DataBinding{Path: d.path})
	}
	if d.fnCall != nil {
		return json.Marshal(d.fnCall)
	}
	return json.Marshal(d.literal)
}

type DynamicBoolean struct {
	literal bool
	set     bool
	path    string
	fnCall  *FunctionCall
}

func LiteralBoolean(b bool) DynamicBoolean {
	return DynamicBoolean{literal: b, set: true}
}

func BoundBoolean(path string) DynamicBoolean {
	return DynamicBoolean{path: path}
}

func (d DynamicBoolean) MarshalJSON() ([]byte, error) {
	if d.path != "" {
		return json.Marshal(&DataBinding{Path: d.path})
	}
	if d.fnCall != nil {
		return json.Marshal(d.fnCall)
	}
	return json.Marshal(d.literal)
}

type DataBinding struct {
	Path string `json:"path"`
}

type FunctionCall struct {
	Call       string         `json:"call"`
	Args       map[string]any `json:"args,omitempty"`
	ReturnType string         `json:"returnType,omitempty"`
}

// Action marshals as either {"event": {"name": "...", "context": {...}}} or {"functionCall": {...}}.
type Action struct {
	Event        *EventActionDef
	FunctionCall *FunctionCall
}

func NewEventAction(name string, context map[string]any) *Action {
	return &Action{Event: &EventActionDef{Name: name, Context: context}}
}

func (a *Action) MarshalJSON() ([]byte, error) {
	if a.Event != nil {
		return json.Marshal(map[string]any{"event": a.Event})
	}
	if a.FunctionCall != nil {
		return json.Marshal(map[string]any{"functionCall": a.FunctionCall})
	}
	return json.Marshal(nil)
}

type EventActionDef struct {
	Name    string         `json:"name"`
	Context map[string]any `json:"context,omitempty"`
}

// --- A2UI Protocol Messages ---

type CreateSurfaceMessage struct {
	Version       string        `json:"version"`
	CreateSurface CreateSurface `json:"createSurface"`
}

type CreateSurface struct {
	SurfaceID     string         `json:"surfaceId"`
	CatalogID     string         `json:"catalogId"`
	Theme         map[string]any `json:"theme,omitempty"`
	SendDataModel bool           `json:"sendDataModel,omitempty"`
}

type UpdateComponentsMessage struct {
	Version          string           `json:"version"`
	UpdateComponents UpdateComponents `json:"updateComponents"`
}

type UpdateComponents struct {
	SurfaceID  string           `json:"surfaceId"`
	Components []map[string]any `json:"components"`
}

type UpdateDataModelMessage struct {
	Version         string           `json:"version"`
	UpdateDataModel UpdateDataModel  `json:"updateDataModel"`
}

type UpdateDataModel struct {
	SurfaceID string         `json:"surfaceId"`
	Path      string         `json:"path,omitempty"`
	Value     map[string]any `json:"value"`
}

type DeleteSurfaceMessage struct {
	Version       string `json:"version"`
	DeleteSurface struct {
		SurfaceID string `json:"surfaceId"`
	} `json:"deleteSurface"`
}
