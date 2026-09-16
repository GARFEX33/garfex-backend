package resourcecore

type (
	KindCode        string
	LifecycleScope  string
	EnumValue       struct{ Value, Label string }
	FieldDescriptor struct {
		Name, Label string
		Kind        ValueKind
		Required    bool
		RefKind     KindCode
		RefScopedBy []string
		// AllowCreate marks a reference field as reuse-before-create: a
		// client should search existing records of RefKind first and offer
		// creating a new one only as a fallback, rather than defaulting
		// straight to a create form.
		AllowCreate bool
		EnumValues  []EnumValue
	}
	CatalogDescriptor struct {
		Kind           KindCode
		Singular       string
		Plural         string
		Fields         []FieldDescriptor
		IdentityFields []string
		ParentKind     KindCode
		ParentField    string
	}
	CatalogRecord struct {
		Kind     KindCode
		ID       int64
		Revision uint64
		Active   bool
		Values   map[string]Value
		Rules    []ApplicabilityRule
	}
	ValueKind string
	Reference struct {
		Kind KindCode
		ID   int64
		Code string
	}
	Value struct {
		Kind      ValueKind
		Text      string
		Bool      bool
		UnitCode  string
		Reference *Reference
		Strings   []string
	}
	ApplicabilityRule struct {
		AttributeCode                               string
		Equals                                      Value
		Mode                                        string
		IdentityParticipates, NotApplicable, Active bool
	}
	ResourceScope            struct{ ClassCode, FamilyCode, TypeCode string }
	CharacteristicDescriptor struct {
		Code, Name string
		ValueType  string
		Dimension  string
	}
	EffectiveAttributeSource struct{ Level, Code string }
	EffectiveAttributeOption struct{ Code, Label string }
	EffectiveAttribute       struct {
		Characteristic       CharacteristicDescriptor
		EffectiveMode        string
		IdentityParticipates bool
		NotApplicable        bool
		Position             int
		HasPosition          bool
		OptionSetCode        string
		Source               EffectiveAttributeSource
		Rules                []ApplicabilityRule
		Options              []EffectiveAttributeOption
	}
	AttributeValue struct {
		Code     string
		Value    Value
		UnitCode string
	}
	Resource struct {
		ID          int64
		IdentityV1  string
		Scope       ResourceScope
		NaturalUnit string
		Active      bool
		Revision    uint64
		Attributes  []AttributeValue
	}
)

const (
	KindClass               KindCode = "CLASE"
	KindFamily              KindCode = "FAMILIA"
	KindType                KindCode = "TIPO"
	KindAttributeDefinition KindCode = "CARACTERISTICA"
	KindOptionSet           KindCode = "CONJUNTO_OPCIONES"
	KindOption              KindCode = "OPCION"
	KindOptionRelation      KindCode = "RELACION_OPCIONES"
	KindUnit                KindCode = "UNIDAD"
	KindUnitPolicy          KindCode = "POLITICA_UNIDAD"
	KindAttributeBinding    KindCode = "APLICABILIDAD"
	KindPresentationField   KindCode = "PRESENTACION"
)

const (
	ScopeActive   LifecycleScope = "ACTIVE"
	ScopeInactive LifecycleScope = "INACTIVE"
	ScopeAll      LifecycleScope = "ALL"
)

const (
	ValueText             ValueKind = "TEXT"
	ValueCode             ValueKind = "CODE"
	ValueBool             ValueKind = "BOOLEAN"
	ValueInteger          ValueKind = "INTEGER"
	ValueDecimal          ValueKind = "DECIMAL"
	ValueQuantity         ValueKind = "QUANTITY"
	ValueReference        ValueKind = "REFERENCE"
	ValueEnum             ValueKind = "ENUM"
	ValueStringList       ValueKind = "STRING_LIST"
	ValueControlledOption ValueKind = "CONTROLLED_OPTION"
	ValueNotApplicable    ValueKind = "NOT_APPLICABLE"
)
