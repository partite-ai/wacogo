package wacogo

import "github.com/partite-ai/wacogo/internal/core"

// EngineOption configures an Engine at construction time. Pass instances
// to NewEngine.
type EngineOption = core.EngineOption

// CoreModuleReplacer inspects a freshly-compiled inline core module and
// optionally returns a replacement. See WithCoreModuleReplacer.
type CoreModuleReplacer = core.CoreModuleReplacer

// Component is an immutable, compiled component-model component ready
// for instantiation. Obtain one from (*Engine).LoadComponent.
type Component = core.Component

// ComponentInstance is a live instance of a Component. Single-threaded:
// the component-model spec forbids concurrent or reentrant use.
type ComponentInstance = core.ComponentInstance

// ComponentType represents a component-model component type. Reserved
// for future use.
type ComponentType = core.ComponentType

// CompiledModule wraps a compiled core wasm module exposed by a
// component as a core-module export.
type CompiledModule = core.CompiledModule

// InstanceType represents a component-model instance type. Reserved
// for future use.
type InstanceType = core.InstanceType

// ExportDesc describes one entry in a Component's exports list.
type ExportDesc = core.ExportDesc

// ImportDesc describes one entry in a Component's imports list.
type ImportDesc = core.ImportDesc

// ExportedFunc is a callable component-model function paired with the
// name it was exported under. Obtain instances from
// (*ComponentInstance).ExportedFunc.
type ExportedFunc = core.ExportedFunc

// InstantiateOption configures a single (*Component).Instantiate call.
// The With* functions in this package construct values of this type.
type InstantiateOption = core.InstantiateOption

// Sort enumerates the kinds of definition a component-model import or
// export can refer to. Values follow the names used in the
// component-model specification.
type Sort = core.Sort

// SortCoreFunc identifies a core-wasm func definition.
const SortCoreFunc = core.SortCoreFunc

// SortCoreTable identifies a core-wasm table definition.
const SortCoreTable = core.SortCoreTable

// SortCoreMemory identifies a core-wasm memory definition.
const SortCoreMemory = core.SortCoreMemory

// SortCoreGlobal identifies a core-wasm global definition.
const SortCoreGlobal = core.SortCoreGlobal

// SortCoreModule identifies a core-wasm module definition.
const SortCoreModule = core.SortCoreModule

// SortCoreInstance identifies a core-wasm instance definition.
const SortCoreInstance = core.SortCoreInstance

// SortFunc identifies a component-level func definition.
const SortFunc = core.SortFunc

// SortValue identifies a component-level value definition.
const SortValue = core.SortValue

// SortType identifies a component-level type definition.
const SortType = core.SortType

// SortComponent identifies a nested-component definition.
const SortComponent = core.SortComponent

// SortInstance identifies a component-level instance definition.
const SortInstance = core.SortInstance

// Type is the interface satisfied by every component-model value type.
type Type = core.Type

// TypeBool is the component-model bool type.
type TypeBool = core.TypeBool

// TypeU8 is the component-model u8 type.
type TypeU8 = core.TypeU8

// TypeU16 is the component-model u16 type.
type TypeU16 = core.TypeU16

// TypeU32 is the component-model u32 type.
type TypeU32 = core.TypeU32

// TypeU64 is the component-model u64 type.
type TypeU64 = core.TypeU64

// TypeS8 is the component-model s8 type.
type TypeS8 = core.TypeS8

// TypeS16 is the component-model s16 type.
type TypeS16 = core.TypeS16

// TypeS32 is the component-model s32 type.
type TypeS32 = core.TypeS32

// TypeS64 is the component-model s64 type.
type TypeS64 = core.TypeS64

// TypeF32 is the component-model f32 type.
type TypeF32 = core.TypeF32

// TypeF64 is the component-model f64 type.
type TypeF64 = core.TypeF64

// TypeChar is the component-model char type.
type TypeChar = core.TypeChar

// TypeString is the component-model string type.
type TypeString = core.TypeString

// TypeList is the component-model list<T> type.
type TypeList = core.TypeList

// TypeTuple is the component-model tuple type.
type TypeTuple = core.TypeTuple

// TypeOption is the component-model option<T> type.
type TypeOption = core.TypeOption

// TypeResult is the component-model result<T,E> type.
type TypeResult = core.TypeResult

// TypeRecord is the component-model record type.
type TypeRecord = core.TypeRecord

// TypeVariant is the component-model variant type.
type TypeVariant = core.TypeVariant

// TypeFlags is the component-model flags type.
type TypeFlags = core.TypeFlags

// TypeEnum is the component-model enum type.
type TypeEnum = core.TypeEnum

// TypeResource is the component-model resource type.
type TypeResource = core.TypeResource

// TypeOwn is the component-model own<R> handle type.
type TypeOwn = core.TypeOwn

// TypeBorrow is the component-model borrow<R> handle type.
type TypeBorrow = core.TypeBorrow

// ResourceHandle is the runtime handle for a resource value as it sits
// in a component instance's resource table.
type ResourceHandle = core.ResourceHandle

// TransferTarget is the destination side of a resource handle
// transfer (LendTo/TransferOwn). Obtain one from a CallContext.
type TransferTarget = core.TransferTarget

// Task is the per-call accounting record shared by the caller and
// callee CallContexts of a single component-model call.
type Task = core.Task

// Field is one named field of a record value.
type Field = core.Field

// FieldType describes one field of a TypeRecord.
type FieldType = core.FieldType

// CaseType describes one case of a TypeVariant.
type CaseType = core.CaseType

// ParamType describes one parameter of a FuncType.
type ParamType = core.ParamType

// ResultType describes one result of a FuncType.
type ResultType = core.ResultType

// FuncType is a component-model function signature.
type FuncType = core.FuncType
