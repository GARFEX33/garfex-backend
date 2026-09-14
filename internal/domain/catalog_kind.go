package domain

// CatalogKindCode identifies one of the 11 administrable catalog-structure
// concepts (design §3) — the record key CatalogRecord/CatalogFilter carry,
// and the registry lookup key for CatalogRegistry.Kind. Design's
// reconciliation note folds spec's "Identidad rule" into
// AttributeBinding.identity_participates (a field, not its own kind), so 11
// kinds cover spec's 10 named administrable concepts.
type CatalogKindCode string

const (
	KindClass               CatalogKindCode = "CLASE"
	KindFamily              CatalogKindCode = "FAMILIA"
	KindType                CatalogKindCode = "TIPO"
	KindAttributeDefinition CatalogKindCode = "CARACTERISTICA"
	KindOptionSet           CatalogKindCode = "CONJUNTO_OPCIONES"
	KindOption              CatalogKindCode = "OPCION"
	KindOptionRelation      CatalogKindCode = "RELACION_OPCIONES"
	KindUnit                CatalogKindCode = "UNIDAD"
	KindUnitPolicy          CatalogKindCode = "POLITICA_UNIDAD"
	KindAttributeBinding    CatalogKindCode = "APLICABILIDAD" // incl. Identidad flag + Rules
	KindPresentationField   CatalogKindCode = "PRESENTACION"
)

// FieldKind is the storage/UI shape of one FieldDescriptor.
type FieldKind uint8

const (
	FieldText FieldKind = iota
	FieldCode
	FieldBool
	FieldInt
	FieldRef
	FieldEnum
	FieldStringList
)

// Immutability is how strongly a FieldDescriptor resists change once set
// (design D11).
type Immutability uint8

const (
	Mutable Immutability = iota
	// ImmutableOnceReferenced marks a "código" field: editable until at
	// least one resource/child references it, then locked (D11 layers 1+2).
	ImmutableOnceReferenced
	ImmutableAlways
)

// EnumValue is one machine value + Spanish label pair for a FieldEnum field
// (e.g. AttributeMode, AttributeValueType).
type EnumValue struct {
	Value string
	Label string
}

// FieldDescriptor describes one editable field of a CatalogKind (design §3).
type FieldDescriptor struct {
	Name        string // storage/record key, e.g. "code"
	Label       string // Spanish, user-visible, e.g. "Código"
	Kind        FieldKind
	Required    bool
	Immutable   Immutability
	Searchable  bool
	Guidance    string          // optional Spanish help shown with the field prompt
	RefKind     CatalogKindCode // when Kind == FieldRef
	RefScopedBy []string        // parent field names narrowing the ref list
	AllowCreate bool            // reuse-before-create: SelectionSearchable + AllowCustom (D12)
	EnumValues  []EnumValue     // when Kind == FieldEnum
}

// RelationDescriptor names one child kind that references this kind, driving
// dependency probes for Desactivar/Eliminar guards (design §6).
type RelationDescriptor struct {
	Kind      CatalogKindCode // child kind
	ViaFields []string        // child field names pointing back at this kind
	Blocking  bool            // a live child blocks Desactivar / physical Delete
}

// CatalogKind is one administrable catalog-structure concept: its Spanish
// labels, its editable fields, its identity key, its place in the
// Clase/Familia/Tipo hierarchy (if any), and the children a guarded
// delete/deactivate must check.
type CatalogKind struct {
	Code           CatalogKindCode
	Singular       string // e.g. "Familia"
	Plural         string // e.g. "Familias"
	Fields         []FieldDescriptor
	IdentityFields []string // uniqueness key, e.g. {"class", "code"}
	ParentKind     CatalogKindCode
	ParentField    string
	// SoftDelete marks whether the kind participates in the complete domain
	// lifecycle. Every registered kind has an Active field and supports
	// deactivate/reactivate alongside create, update, and delete.
	SoftDelete bool
	Children   []RelationDescriptor
}

// CatalogRegistry is the fixed, in-memory set of every administrable
// CatalogKind (design §3) — no table introspection; only registered kinds
// are administrable.
type CatalogRegistry struct{ kinds []CatalogKind }

// NewCatalogRegistry builds the registry of all 11 CatalogKinds.
func NewCatalogRegistry() CatalogRegistry {
	return CatalogRegistry{kinds: catalogKinds()}
}

// Kinds returns every registered CatalogKind, in a defensive copy — mutating
// the returned slice/elements never affects the registry's own state.
func (r CatalogRegistry) Kinds() []CatalogKind {
	out := make([]CatalogKind, len(r.kinds))
	copy(out, r.kinds)
	return out
}

// Kind looks up one CatalogKind by its code.
func (r CatalogRegistry) Kind(code CatalogKindCode) (CatalogKind, bool) {
	for _, kind := range r.kinds {
		if kind.Code == code {
			return kind, true
		}
	}
	return CatalogKind{}, false
}

func codeField() FieldDescriptor {
	return FieldDescriptor{Name: "code", Label: "Código", Kind: FieldCode, Required: true, Immutable: ImmutableOnceReferenced, Searchable: true}
}

func attributeValueTypeEnum() []EnumValue {
	return []EnumValue{
		{Value: "CONTROLLED_OPTION", Label: "Opción controlada"},
		{Value: "INTEGER", Label: "Entero"},
		{Value: "DECIMAL", Label: "Decimal"},
		{Value: "QUANTITY", Label: "Cantidad"},
		{Value: "BOOLEAN", Label: "Booleano"},
		{Value: "CONTROLLED_TEXT", Label: "Texto controlado"},
	}
}

func attributeModeEnum() []EnumValue {
	return []EnumValue{
		{Value: "REQUIRED", Label: "Requerido"},
		{Value: "OPTIONAL", Label: "Opcional"},
		{Value: "CONDITIONAL", Label: "Condicional"},
		{Value: "FORBIDDEN", Label: "Prohibido"},
	}
}

func catalogKinds() []CatalogKind {
	return []CatalogKind{
		{
			Code: KindClass, Singular: "Clase", Plural: "Clases",
			Fields: []FieldDescriptor{
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
				{Name: "plural", Label: "Plural", Kind: FieldText, Required: true},
				{Name: "slug", Label: "Slug", Kind: FieldText, Required: true},
				{Name: "order", Label: "Orden", Kind: FieldInt},
				{Name: "aliases", Label: "Alias", Kind: FieldStringList},
				{Name: "keywords", Label: "Palabras clave", Kind: FieldStringList},
			},
			IdentityFields: []string{"code"},
			SoftDelete:     true,
			Children: []RelationDescriptor{
				{Kind: KindFamily, ViaFields: []string{"class"}, Blocking: true},
			},
		},
		{
			Code: KindFamily, Singular: "Familia", Plural: "Familias",
			Fields: []FieldDescriptor{
				{Name: "class", Label: "Clase", Kind: FieldRef, Required: true, RefKind: KindClass},
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
			},
			IdentityFields: []string{"class", "code"},
			ParentKind:     KindClass, ParentField: "class",
			SoftDelete: true,
			Children: []RelationDescriptor{
				{Kind: KindType, ViaFields: []string{"family"}, Blocking: true},
				{Kind: KindAttributeBinding, ViaFields: []string{"family"}, Blocking: true},
				{Kind: KindUnitPolicy, ViaFields: []string{"family"}, Blocking: true},
			},
		},
		{
			Code: KindType, Singular: "Tipo", Plural: "Tipos",
			Fields: []FieldDescriptor{
				{Name: "class", Label: "Clase", Kind: FieldRef, Required: true, RefKind: KindClass},
				{Name: "family", Label: "Familia", Kind: FieldRef, Required: true, RefKind: KindFamily, RefScopedBy: []string{"class"}},
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
			},
			IdentityFields: []string{"class", "family", "code"},
			ParentKind:     KindFamily, ParentField: "family",
			SoftDelete: true,
			Children: []RelationDescriptor{
				{Kind: KindAttributeBinding, ViaFields: []string{"type"}, Blocking: true},
				{Kind: KindPresentationField, ViaFields: []string{"type"}, Blocking: false},
			},
		},
		{
			Code: KindAttributeDefinition, Singular: "Característica", Plural: "Características",
			Fields: []FieldDescriptor{
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
				{Name: "valueType", Label: "Tipo de valor", Kind: FieldEnum, Required: true, EnumValues: attributeValueTypeEnum()},
				{Name: "dimension", Label: "Dimensión", Kind: FieldText},
				{Name: "defaultIdentityParticipates", Label: "Participa en Identidad por defecto", Kind: FieldBool},
			},
			IdentityFields: []string{"code"},
			SoftDelete:     true,
			Children: []RelationDescriptor{
				{Kind: KindAttributeBinding, ViaFields: []string{"characteristic"}, Blocking: true},
				{Kind: KindOption, ViaFields: []string{"characteristic"}, Blocking: true},
			},
		},
		{
			Code: KindOptionSet, Singular: "Conjunto de Opciones", Plural: "Conjuntos de Opciones",
			Fields: []FieldDescriptor{
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
			},
			IdentityFields: []string{"code"},
			SoftDelete:     true,
			Children: []RelationDescriptor{
				{Kind: KindOption, ViaFields: []string{"optionSet"}, Blocking: true},
				{Kind: KindOptionRelation, ViaFields: []string{"optionSet"}, Blocking: true},
			},
		},
		{
			Code: KindOption, Singular: "Opción", Plural: "Opciones",
			Fields: []FieldDescriptor{
				{Name: "optionSet", Label: "Conjunto de Opciones", Kind: FieldRef, Required: true, RefKind: KindOptionSet, AllowCreate: true},
				{Name: "characteristic", Label: "Característica", Kind: FieldRef, Required: true, RefKind: KindAttributeDefinition, AllowCreate: true},
				codeField(),
				{Name: "label", Label: "Etiqueta", Kind: FieldText, Required: true, Searchable: true},
			},
			IdentityFields: []string{"optionSet", "characteristic", "code"},
			ParentKind:     KindOptionSet, ParentField: "optionSet",
			SoftDelete: true,
			Children: []RelationDescriptor{
				{Kind: KindOptionRelation, ViaFields: []string{"fromOption", "toOption"}, Blocking: true},
			},
		},
		{
			Code: KindOptionRelation, Singular: "Relación de Opciones", Plural: "Relaciones de Opciones",
			Fields: []FieldDescriptor{
				{Name: "optionSet", Label: "Conjunto de Opciones", Kind: FieldRef, Required: true, RefKind: KindOptionSet},
				{Name: "fromCharacteristic", Label: "Característica origen", Kind: FieldRef, Required: true, RefKind: KindAttributeDefinition},
				{Name: "fromOption", Label: "Opción origen", Kind: FieldRef, Required: true, RefKind: KindOption, RefScopedBy: []string{"optionSet", "fromCharacteristic"}},
				{Name: "toCharacteristic", Label: "Característica destino", Kind: FieldRef, Required: true, RefKind: KindAttributeDefinition},
				{Name: "toOption", Label: "Opción destino", Kind: FieldRef, Required: true, RefKind: KindOption, RefScopedBy: []string{"optionSet", "toCharacteristic"}},
			},
			IdentityFields: []string{"optionSet", "fromOption", "toOption"},
			ParentKind:     KindOptionSet, ParentField: "optionSet",
			SoftDelete: true,
		},
		{
			Code: KindUnit, Singular: "Unidad", Plural: "Unidades",
			Fields: []FieldDescriptor{
				codeField(),
				{Name: "name", Label: "Nombre", Kind: FieldText, Required: true, Searchable: true},
				{Name: "symbol", Label: "Símbolo", Kind: FieldText, Required: true},
				{Name: "dimension", Label: "Dimensión", Kind: FieldText, Required: true, Guidance: "Categoría de medición. Ejemplos: Longitud, Masa, Tiempo o Pieza."},
			},
			IdentityFields: []string{"code"},
			SoftDelete:     true,
			Children: []RelationDescriptor{
				{Kind: KindUnitPolicy, ViaFields: []string{"unit"}, Blocking: true},
			},
		},
		{
			Code: KindUnitPolicy, Singular: "Política de Unidad", Plural: "Políticas de Unidad",
			Fields: []FieldDescriptor{
				{Name: "class", Label: "Clase", Kind: FieldRef, Required: true, RefKind: KindClass},
				{Name: "family", Label: "Familia", Kind: FieldRef, Required: true, RefKind: KindFamily, RefScopedBy: []string{"class"}},
				{Name: "unit", Label: "Unidad", Kind: FieldRef, Required: true, RefKind: KindUnit, AllowCreate: true},
				{Name: "allowed", Label: "Permitida", Kind: FieldBool},
				{Name: "suggested", Label: "Sugerida", Kind: FieldBool},
			},
			IdentityFields: []string{"class", "family", "unit"},
			ParentKind:     KindFamily, ParentField: "family",
			SoftDelete: true,
		},
		{
			Code: KindAttributeBinding, Singular: "Aplicabilidad", Plural: "Aplicabilidades",
			Fields: []FieldDescriptor{
				{Name: "class", Label: "Clase", Kind: FieldRef, Required: true, RefKind: KindClass},
				{Name: "family", Label: "Familia", Kind: FieldRef, Required: true, RefKind: KindFamily, RefScopedBy: []string{"class"}},
				{Name: "type", Label: "Tipo", Kind: FieldRef, RefKind: KindType, RefScopedBy: []string{"class", "family"}},
				{Name: "characteristic", Label: "Característica", Kind: FieldRef, Required: true, RefKind: KindAttributeDefinition, AllowCreate: true},
				{Name: "optionSet", Label: "Conjunto de Opciones", Kind: FieldRef, RefKind: KindOptionSet, AllowCreate: true},
				{Name: "mode", Label: "Modo", Kind: FieldEnum, Required: true, EnumValues: attributeModeEnum()},
				{Name: "identityParticipates", Label: "Participa en Identidad", Kind: FieldBool},
			},
			IdentityFields: []string{"class", "family", "type", "characteristic"},
			ParentKind:     KindFamily, ParentField: "family",
			SoftDelete: true,
		},
		{
			Code: KindPresentationField, Singular: "Campo de Presentación", Plural: "Campos de Presentación",
			Fields: []FieldDescriptor{
				{Name: "class", Label: "Clase", Kind: FieldRef, Required: true, RefKind: KindClass},
				{Name: "family", Label: "Familia", Kind: FieldRef, Required: true, RefKind: KindFamily, RefScopedBy: []string{"class"}},
				{Name: "type", Label: "Tipo", Kind: FieldRef, Required: true, RefKind: KindType, RefScopedBy: []string{"class", "family"}},
				{Name: "characteristic", Label: "Característica", Kind: FieldRef, Required: true, RefKind: KindAttributeDefinition},
				{Name: "position", Label: "Posición", Kind: FieldInt, Required: true},
			},
			IdentityFields: []string{"class", "family", "type", "characteristic"},
			ParentKind:     KindType, ParentField: "type",
			SoftDelete: true,
		},
	}
}
