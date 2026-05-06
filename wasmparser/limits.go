package wasmparser

const (
	// Core limits
	MaxTypes           = 1_000_000
	MaxFunctions       = 1_000_000
	MaxImports         = 1_000_000
	MaxExports         = 1_000_000
	MaxGlobals         = 1_000_000
	MaxTables          = 100
	MaxMemories        = 100
	MaxTags            = 1_000_000
	MaxStringSize      = 100_000
	MaxFunctionParams  = 1_000
	MaxFunctionResults = 1_000

	// Component model limits
	MaxModuleSize                = 1 << 30 // 1 GiB
	MaxModuleTypeDeclarations    = 100_000
	MaxComponentTypeDeclarations = 1_000_000
	MaxInstanceTypeDeclarations  = 1_000_000
	MaxRecordFields              = 10_000
	MaxVariantCases              = 10_000
	MaxTupleTypes                = 10_000
	MaxFlagNames                 = 1_000
	MaxEnumCases                 = 10_000
	MaxInstantiationArgs         = 100_000
	MaxCanonicalOptions          = 10
	MaxStartArgs                 = 1_000
	MaxModules                   = 1_000
	MaxComponents                = 1_000
	MaxInstances                 = 1_000
	MaxValues                    = 1_000
	MaxComponentExternNames      = 100_000
	MaxEffectiveTypeSize         = 1_000_000
)
