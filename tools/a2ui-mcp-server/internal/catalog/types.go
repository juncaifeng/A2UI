package catalog

// ComponentDef represents a parsed A2UI component definition.
type ComponentDef struct {
	Name       string
	Properties map[string]PropertyDef
	Required   []string
	Desc       string
}

// PropertyDef represents a single property of a component.
type PropertyDef struct {
	Name        string
	Type        string   // "string", "number", "boolean", "array", "object"
	Ref         string   // resolved ref name: "DynamicString", "ChildList", "Action", "ComponentId", etc.
	Description string
	Enum        []string
	Default     any
	// Flags derived from ref analysis
	IsDynamicString     bool // ref=DynamicString
	IsDynamicNumber     bool // ref=DynamicNumber
	IsDynamicBoolean    bool // ref=DynamicBoolean
	IsDynamicStringList bool // ref=DynamicStringList
	IsChildList      bool // ref=ChildList
	IsComponentId    bool // ref=ComponentId
	IsAction         bool // ref=Action
	IsConst          bool // type=const (e.g. "component": "Text")
	ConstValue       string
	// For array items with object properties
	ItemsProperties map[string]PropertyDef
	ItemsRequired   []string
}

// Catalog holds all parsed component and function definitions.
type Catalog struct {
	Components map[string]*ComponentDef
	Functions  map[string]*FunctionDef
	CatalogID  string
}

// FunctionDef represents a parsed A2UI client-side function.
type FunctionDef struct {
	Name        string
	Description string
	Args        map[string]PropertyDef
	Required    []string
	ReturnType  string
}

// IsRequired checks if a property is in the component's required list.
func (c *ComponentDef) IsRequired(propName string) bool {
	for _, r := range c.Required {
		if r == propName {
			return true
		}
	}
	return false
}

// MergeCatalogs combines multiple catalogs into a single Catalog.
// Later catalogs override earlier ones when component names collide.
// Functions are merged similarly.
func MergeCatalogs(catalogs []*Catalog) *Catalog {
	merged := &Catalog{
		Components: make(map[string]*ComponentDef),
		Functions:  make(map[string]*FunctionDef),
	}

	var catalogIDs []string
	for _, cat := range catalogs {
		if cat.CatalogID != "" {
			catalogIDs = append(catalogIDs, cat.CatalogID)
		}
		for name, comp := range cat.Components {
			merged.Components[name] = comp
		}
		for name, fn := range cat.Functions {
			merged.Functions[name] = fn
		}
	}

	// Store all catalog IDs separated by comma
	if len(catalogIDs) > 0 {
		merged.CatalogID = catalogIDs[0]
		for _, id := range catalogIDs[1:] {
			merged.CatalogID += "," + id
		}
	}

	return merged
}
