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
	MaxFlagNames                 = 32
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

	// MaxNestingDepth bounds recursive parsing of component/instance type
	// declarations and nested component sections to prevent stack overflow
	// from adversarial input.
	MaxNestingDepth = 100

	// MaxFunctionBodySize bounds a single core function body so adversarial
	// size fields cannot drive multi-GB allocations.
	MaxFunctionBodySize = 16 * 1024 * 1024 // 16 MiB
)

// checkSectionLength returns length as int after verifying it fits within
// MaxModuleSize. This is the canonical guard for adversarial u32 lengths
// before they reach `make([]byte, n)`. It is also 32-bit-Go safe: any value
// large enough to wrap int's signed range trips the cap first.
func checkSectionLength(offset uint64, length uint32, what string) (int, error) {
	if uint64(length) > MaxModuleSize {
		return 0, errfAt(offset, "%s length %d exceeds maximum %d", what, length, MaxModuleSize)
	}
	return int(length), nil
}

// checkCount verifies count ≤ max.
func checkCount(offset uint64, count, max uint32, what string) error {
	if count > max {
		return errfAt(offset, "%s count %d exceeds maximum %d", what, count, max)
	}
	return nil
}
