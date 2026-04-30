package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads basic_catalog.json and common_types.json from specDir and returns a parsed Catalog.
func Load(specDir string) (*Catalog, error) {
	catalogPath := filepath.Join(specDir, "basic_catalog.json")
	commonPath := filepath.Join(specDir, "common_types.json")

	catalogData, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	commonData, err := os.ReadFile(commonPath)
	if err != nil {
		return nil, fmt.Errorf("read common types: %w", err)
	}

	var rawCatalog map[string]json.RawMessage
	if err := json.Unmarshal(catalogData, &rawCatalog); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	var rawCommon map[string]json.RawMessage
	if err := json.Unmarshal(commonData, &rawCommon); err != nil {
		return nil, fmt.Errorf("parse common types: %w", err)
	}

	// Extract common type definitions
	commonDefs := parseDefs(rawCommon)

	// Parse catalog ID
	var catalogID string
	if raw, ok := rawCatalog["catalogId"]; ok {
		_ = json.Unmarshal(raw, &catalogID)
	}

	// Parse components
	var rawComponents map[string]json.RawMessage
	if raw, ok := rawCatalog["components"]; ok {
		_ = json.Unmarshal(raw, &rawComponents)
	}

	components := make(map[string]*ComponentDef, len(rawComponents))
	for name, rawComp := range rawComponents {
		comp, err := parseComponent(name, rawComp, commonDefs)
		if err != nil {
			return nil, fmt.Errorf("parse component %s: %w", name, err)
		}
		components[name] = comp
	}

	// Parse functions (optional)
	functions := make(map[string]*FunctionDef)
	if raw, ok := rawCatalog["functions"]; ok {
		var rawFuncs map[string]json.RawMessage
		_ = json.Unmarshal(raw, &rawFuncs)
		for name, rawFunc := range rawFuncs {
			fn, err := parseFunction(name, rawFunc)
			if err == nil {
				functions[name] = fn
			}
		}
	}

	return &Catalog{
		Components: components,
		Functions:  functions,
		CatalogID:  catalogID,
	}, nil
}

func parseDefs(raw map[string]json.RawMessage) map[string]json.RawMessage {
	var defs map[string]json.RawMessage
	if d, ok := raw["$defs"]; ok {
		_ = json.Unmarshal(d, &defs)
	}
	return defs
}

func parseComponent(name string, raw json.RawMessage, commonDefs map[string]json.RawMessage) (*ComponentDef, error) {
	comp := &ComponentDef{
		Name:       name,
		Properties: make(map[string]PropertyDef),
	}

	var node map[string]any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}

	// Handle allOf: merge properties from each element
	if allOf, ok := node["allOf"].([]any); ok {
		for _, item := range allOf {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			// Check for $ref
			if ref, ok := itemMap["$ref"].(string); ok {
				resolveRefInto(comp, ref, commonDefs)
				continue
			}
			// Merge properties from this element
			mergeProperties(comp, itemMap)
		}
	}

	// Handle direct properties (no allOf)
	if _, hasAllOf := node["allOf"]; !hasAllOf {
		mergeProperties(comp, node)
	}

	// Extract description
	if desc, ok := node["description"].(string); ok {
		comp.Desc = desc
	}

	return comp, nil
}

func resolveRefInto(comp *ComponentDef, ref string, commonDefs map[string]json.RawMessage) {
	// Parse refs like "common_types.json#/$defs/ComponentCommon" or "#/$defs/CatalogComponentCommon"
	parts := strings.SplitN(ref, "#/$defs/", 2)
	if len(parts) < 2 {
		return
	}
	typeName := parts[1]

	// Look up in common defs
	rawDef, ok := commonDefs[typeName]
	if !ok {
		return
	}

	var def map[string]any
	if err := json.Unmarshal(rawDef, &def); err != nil {
		return
	}

	mergeProperties(comp, def)
}

func mergeProperties(comp *ComponentDef, node map[string]any) {
	props, _ := node["properties"].(map[string]any)
	required, _ := node["required"].([]any)

	for _, r := range required {
		if s, ok := r.(string); ok {
			comp.Required = append(comp.Required, s)
		}
	}

	for propName, propVal := range props {
		propMap, ok := propVal.(map[string]any)
		if !ok {
			continue
		}
		pd := parsePropertyDef(propName, propMap)
		comp.Properties[propName] = pd
	}
}

func parsePropertyDef(name string, m map[string]any) PropertyDef {
	pd := PropertyDef{
		Name: name,
	}

	// Check for const
	if cv, ok := m["const"].(string); ok {
		pd.IsConst = true
		pd.ConstValue = cv
		pd.Type = "const"
		return pd
	}

	// Check for $ref
	if ref, ok := m["$ref"].(string); ok {
		pd.Ref = resolveRefName(ref)
		classifyRef(&pd)
	}

	// Check for oneOf with $ref inside
	if oneOf, ok := m["oneOf"].([]any); ok {
		pd.Ref = resolveOneOfRef(oneOf)
		if pd.Ref != "" {
			classifyRef(&pd)
		}
	}

	// Check for allOf with $ref inside
	if allOf, ok := m["allOf"].([]any); ok {
		for _, item := range allOf {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if ref, ok := itemMap["$ref"].(string); ok && pd.Ref == "" {
				pd.Ref = resolveRefName(ref)
				classifyRef(&pd)
			}
		}
	}

	// Basic type
	if t, ok := m["type"].(string); ok {
		pd.Type = t
	}
	if pd.Type == "" && pd.Ref == "" {
		pd.Type = "string" // default fallback
	}

	// Description
	if desc, ok := m["description"].(string); ok {
		pd.Description = desc
	}

	// Enum
	if enum, ok := m["enum"].([]any); ok {
		for _, e := range enum {
			if s, ok := e.(string); ok {
				pd.Enum = append(pd.Enum, s)
			}
		}
	}

	// Default
	if def, ok := m["default"]; ok {
		pd.Default = def
	}

	// Array items with object properties
	if items, ok := m["items"].(map[string]any); ok {
		if itemProps, ok := items["properties"].(map[string]any); ok {
			pd.ItemsProperties = make(map[string]PropertyDef)
			for k, v := range itemProps {
				if pm, ok := v.(map[string]any); ok {
					pd.ItemsProperties[k] = parsePropertyDef(k, pm)
				}
			}
		}
		if itemReq, ok := items["required"].([]any); ok {
			for _, r := range itemReq {
				if s, ok := r.(string); ok {
					pd.ItemsRequired = append(pd.ItemsRequired, s)
				}
			}
		}
	}

	return pd
}

func classifyRef(pd *PropertyDef) {
	switch pd.Ref {
	case "DynamicString", "DynamicValue":
		pd.IsDynamicString = true
		pd.Type = "string"
	case "DynamicNumber":
		pd.IsDynamicNumber = true
		pd.Type = "number"
	case "DynamicBoolean":
		pd.IsDynamicBoolean = true
		pd.Type = "boolean"
	case "DynamicStringList":
		pd.IsDynamicStringList = true
		pd.Type = "array"
	case "ChildList":
		pd.IsChildList = true
		pd.Type = "array"
	case "ComponentId":
		pd.IsComponentId = true
		pd.Type = "string"
	case "Action":
		pd.IsAction = true
		pd.Type = "object"
	case "CheckRule":
		pd.Type = "array"
	}
}

func resolveRefName(ref string) string {
	// "common_types.json#/$defs/DynamicString" → "DynamicString"
	// "#/$defs/CatalogComponentCommon" → "CatalogComponentCommon"
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ref
}

func resolveOneOfRef(oneOf []any) string {
	for _, item := range oneOf {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ref, ok := m["$ref"].(string); ok {
			return resolveRefName(ref)
		}
	}
	return ""
}

func parseFunction(name string, raw json.RawMessage) (*FunctionDef, error) {
	var node map[string]any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}

	fn := &FunctionDef{Name: name}
	if desc, ok := node["description"].(string); ok {
		fn.Description = desc
	}

	if argsNode, ok := node["args"].(map[string]any); ok {
		if props, ok := argsNode["properties"].(map[string]any); ok {
			fn.Args = make(map[string]PropertyDef)
			for k, v := range props {
				if pm, ok := v.(map[string]any); ok {
					fn.Args[k] = parsePropertyDef(k, pm)
				}
			}
		}
		if req, ok := argsNode["required"].([]any); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					fn.Required = append(fn.Required, s)
				}
			}
		}
	}

	// Extract returnType from properties.returnType.const
	if props, ok := node["properties"].(map[string]any); ok {
		if rt, ok := props["returnType"].(map[string]any); ok {
			if c, ok := rt["const"].(string); ok {
				fn.ReturnType = c
			}
		}
	}

	return fn, nil
}

// LoadFromCatalogFile loads a single catalog from a catalog file path,
// resolving common_types.json relative to the catalog's $ref paths.
func LoadFromCatalogFile(catalogPath string) (*Catalog, error) {
	catalogData, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}

	var rawCatalog map[string]json.RawMessage
	if err := json.Unmarshal(catalogData, &rawCatalog); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}

	// Find the common_types.json path by looking at the first $ref
	commonPath := findCommonTypesPath(rawCatalog, catalogPath)
	if commonPath == "" {
		// No $ref to common_types.json; catalog may be self-contained
		commonPath = filepath.Join(filepath.Dir(catalogPath), "common_types.json")
	}

	commonDefs := map[string]json.RawMessage{}
	if data, err := os.ReadFile(commonPath); err == nil {
		var rawCommon map[string]json.RawMessage
		if err := json.Unmarshal(data, &rawCommon); err == nil {
			commonDefs = parseDefs(rawCommon)
		}
	}

	// Parse catalog ID
	var catalogID string
	if raw, ok := rawCatalog["catalogId"]; ok {
		_ = json.Unmarshal(raw, &catalogID)
	}

	// Parse components
	var rawComponents map[string]json.RawMessage
	if raw, ok := rawCatalog["components"]; ok {
		_ = json.Unmarshal(raw, &rawComponents)
	}

	components := make(map[string]*ComponentDef, len(rawComponents))
	for name, rawComp := range rawComponents {
		comp, err := parseComponent(name, rawComp, commonDefs)
		if err != nil {
			return nil, fmt.Errorf("parse component %s: %w", name, err)
		}
		components[name] = comp
	}

	// Parse functions (optional)
	functions := make(map[string]*FunctionDef)
	if raw, ok := rawCatalog["functions"]; ok {
		var rawFuncs map[string]json.RawMessage
		_ = json.Unmarshal(raw, &rawFuncs)
		for name, rawFunc := range rawFuncs {
			fn, err := parseFunction(name, rawFunc)
			if err == nil {
				functions[name] = fn
			}
		}
	}

	return &Catalog{
		Components: components,
		Functions:  functions,
		CatalogID:  catalogID,
	}, nil
}

// LoadAll scans specDir recursively for *_catalog.json files and loads each one.
func LoadAll(specDir string) ([]*Catalog, error) {
	var catalogPaths []string
	err := filepath.Walk(specDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), "_catalog.json") {
			catalogPaths = append(catalogPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan for catalog files: %w", err)
	}

	if len(catalogPaths) == 0 {
		return nil, fmt.Errorf("no *_catalog.json files found in %s", specDir)
	}

	var catalogs []*Catalog
	for _, path := range catalogPaths {
		cat, err := LoadFromCatalogFile(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		catalogs = append(catalogs, cat)
	}

	return catalogs, nil
}

// findCommonTypesPath searches the catalog JSON for a $ref referencing
// common_types.json and returns the resolved absolute path.
// It looks for patterns like "../../common_types.json#/$defs/..." or
// "common_types.json#/$defs/...".
func findCommonTypesPath(rawCatalog map[string]json.RawMessage, catalogPath string) string {
	// Search components for $ref patterns
	var rawComponents map[string]json.RawMessage
	if raw, ok := rawCatalog["components"]; ok {
		_ = json.Unmarshal(raw, &rawComponents)
	}

	for _, rawComp := range rawComponents {
		if path := findRefInValue(rawComp, catalogPath); path != "" {
			return path
		}
	}
	return ""
}

// findRefInValue recursively searches a JSON value for a $ref containing "common_types.json".
func findRefInValue(raw json.RawMessage, catalogPath string) string {
	var val any
	if err := json.Unmarshal(raw, &val); err != nil {
		return ""
	}
	return searchForCommonRef(val, catalogPath)
}

func searchForCommonRef(val any, catalogPath string) string {
	switch v := val.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			if resolved := resolveCommonTypesPath(ref, catalogPath); resolved != "" {
				return resolved
			}
		}
		for _, child := range v {
			if result := searchForCommonRef(child, catalogPath); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range v {
			if result := searchForCommonRef(child, catalogPath); result != "" {
				return result
			}
		}
	}
	return ""
}

// resolveCommonTypesPath resolves a $ref like "../../common_types.json#/$defs/DynamicString"
// to an absolute file path.
func resolveCommonTypesPath(ref string, catalogPath string) string {
	// Split at # to get the file part
	filePart := strings.SplitN(ref, "#", 2)[0]
	if filePart == "" || !strings.Contains(filePart, "common_types.json") {
		return ""
	}
	// Resolve relative to catalog file directory
	catalogDir := filepath.Dir(catalogPath)
	absPath := filepath.Join(catalogDir, filePart)
	// Clean path (resolve ../ etc.)
	return filepath.Clean(absPath)
}
