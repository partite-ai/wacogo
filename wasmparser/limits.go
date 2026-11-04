package wasmparser

import (
	"cmp"
	"errors"
	"math"
)

// ErrInstanceLimitExceeded is wrapped by the error reported when one
// instantiation of a component would create more instances than ParseLimits
// allows.
var ErrInstanceLimitExceeded = errors.New("instance limit exceeded")

// Unlimited is the ParseLimits value for a limit that is not enforced.
const Unlimited uint32 = math.MaxUint32

// maxFlagNames is the spec's limit on the names in a flags type.
const maxFlagNames = 32

// ParseLimits bounds what parsing and validation accept. A zero field means the
// default given for it, and Unlimited means no limit.
//
// Raising a limit raises what a binary can make the parser allocate from a small
// input, as many of these bound counts that are allocated for before their
// contents are read. Only input that is trusted should be parsed with limits
// raised far beyond their defaults.
type ParseLimits struct {
	// Core module limits.

	Types           uint32 // types in a module; default 1,000,000
	Functions       uint32 // functions in a module, and locals in a function; default 1,000,000
	Imports         uint32 // imports in a module; default 1,000,000
	Exports         uint32 // exports in a module or core instance; default 1,000,000
	Globals         uint32 // globals in a module; default 1,000,000
	Tables          uint32 // tables in a module; default 100
	Memories        uint32 // memories in a module; default 100
	FunctionResults uint32 // results of a core function; default 1,000

	// FunctionBodySize is the size in bytes of a function body; default 16 MiB.
	FunctionBodySize uint32

	// FunctionParams is the parameters of a core or component function; default
	// 1,000.
	FunctionParams uint32

	// Component limits.

	ModuleTypeDeclarations    uint32 // declarations in a module type; default 100,000
	ComponentTypeDeclarations uint32 // declarations in a component type; default 1,000,000
	InstanceTypeDeclarations  uint32 // declarations in an instance type; default 1,000,000
	RecordFields              uint32 // fields in a record; default 10,000
	VariantCases              uint32 // cases in a variant; default 10,000
	TupleTypes                uint32 // types in a tuple; default 10,000
	EnumCases                 uint32 // cases in an enum; default 10,000
	InstantiationArgs         uint32 // arguments to an instantiation; default 100,000
	CanonicalOptions          uint32 // options on a canonical function; default 10
	StartArgs                 uint32 // arguments to a start function; default 1,000
	ComponentExternNames      uint32 // exports of an inline component instance; default 100,000
	EffectiveTypeSize         uint32 // size of a type with its references expanded; default 1,000,000

	// StringSize is the size in bytes of a string; default 100,000.
	StringSize uint32

	// ModuleSize is the size in bytes of a section; default 1 GiB.
	ModuleSize uint32

	// NestingDepth is the depth of nested components, modules and type
	// declarations; default 100.
	NestingDepth uint32

	// CoreInstances and ComponentInstances bound the instances one instantiation
	// of a component creates, counting the component itself and everything nested
	// inside it; default 1,000 each. Instances inside a component it imports are
	// not counted, as they depend on what is supplied.
	//
	// The time taken to check is proportional to the instances counted, and
	// instantiating creates them all, so with both Unlimited a small binary can make
	// instantiation take practically forever.
	CoreInstances      uint32
	ComponentInstances uint32
}

// resolved returns l with each zero field set to its default.
func (l ParseLimits) resolved() ParseLimits {
	return ParseLimits{
		Types:            cmp.Or(l.Types, 1_000_000),
		Functions:        cmp.Or(l.Functions, 1_000_000),
		Imports:          cmp.Or(l.Imports, 1_000_000),
		Exports:          cmp.Or(l.Exports, 1_000_000),
		Globals:          cmp.Or(l.Globals, 1_000_000),
		Tables:           cmp.Or(l.Tables, 100),
		Memories:         cmp.Or(l.Memories, 100),
		FunctionResults:  cmp.Or(l.FunctionResults, 1_000),
		FunctionBodySize: cmp.Or(l.FunctionBodySize, 16*1024*1024),
		FunctionParams:   cmp.Or(l.FunctionParams, 1_000),

		ModuleTypeDeclarations:    cmp.Or(l.ModuleTypeDeclarations, 100_000),
		ComponentTypeDeclarations: cmp.Or(l.ComponentTypeDeclarations, 1_000_000),
		InstanceTypeDeclarations:  cmp.Or(l.InstanceTypeDeclarations, 1_000_000),
		RecordFields:              cmp.Or(l.RecordFields, 10_000),
		VariantCases:              cmp.Or(l.VariantCases, 10_000),
		TupleTypes:                cmp.Or(l.TupleTypes, 10_000),
		EnumCases:                 cmp.Or(l.EnumCases, 10_000),
		InstantiationArgs:         cmp.Or(l.InstantiationArgs, 100_000),
		CanonicalOptions:          cmp.Or(l.CanonicalOptions, 10),
		StartArgs:                 cmp.Or(l.StartArgs, 1_000),
		ComponentExternNames:      cmp.Or(l.ComponentExternNames, 100_000),
		EffectiveTypeSize:         cmp.Or(l.EffectiveTypeSize, 1_000_000),
		StringSize:                cmp.Or(l.StringSize, 100_000),
		ModuleSize:                cmp.Or(l.ModuleSize, 1<<30),
		NestingDepth:              cmp.Or(l.NestingDepth, 100),
		CoreInstances:             cmp.Or(l.CoreInstances, 1_000),
		ComponentInstances:        cmp.Or(l.ComponentInstances, 1_000),
	}
}

var defaultParseLimits = ParseLimits{}.resolved()

// ParseOption configures a Parser, Validator or ValidatingParser.
type ParseOption func(*ParseLimits)

// WithParseLimits sets the limits to parse or validate with.
func WithParseLimits(limits ParseLimits) ParseOption {
	return func(l *ParseLimits) { *l = limits }
}

func newParseLimits(opts []ParseOption) *ParseLimits {
	var l ParseLimits
	for _, opt := range opts {
		opt(&l)
	}
	l = l.resolved()
	return &l
}

// checkSectionLength returns length as int after verifying it fits within max.
// This is the canonical guard for adversarial u32 lengths before they reach
// `make([]byte, n)`.
func checkSectionLength(offset uint64, length, max uint32, what string) (int, error) {
	if length > max {
		return 0, errfAt(offset, "%s length %d exceeds maximum %d", what, length, max)
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
