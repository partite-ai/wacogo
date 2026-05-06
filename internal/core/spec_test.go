package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/partite-ai/wacogo/internal/core"
	"github.com/partite-ai/wacogo/internal/spectest"
	"github.com/partite-ai/wacogo/wasmparser"
)

// --- JSON types for wasm-tools json-from-wast output ---

type specJSON struct {
	SourceFilename string        `json:"source_filename"`
	Commands       []specCommand `json:"commands"`
}

type specCommand struct {
	Type       string      `json:"type"`
	Line       int         `json:"line"`
	Filename   string      `json:"filename,omitempty"`
	ModuleType string      `json:"module_type,omitempty"`
	Text       string      `json:"text,omitempty"`
	Name       string      `json:"name,omitempty"`
	Action     *specAction `json:"action,omitempty"`
	Expected   []specValue `json:"expected,omitempty"`

	// module_instance fields
	Instance string `json:"instance,omitempty"`
	Module   string `json:"module,omitempty"`
}

type specAction struct {
	Type   string      `json:"type"`
	Module string      `json:"module,omitempty"`
	Field  string      `json:"field"`
	Args   []specValue `json:"args,omitempty"`
}

type specValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// --- Test runner state ---

type specTestState struct {
	t      *testing.T
	ctx    context.Context
	engine *core.Engine

	// Named module definitions (not yet instantiated)
	definitions map[string]*core.Component

	// importOpts is the accumulated list of core.InstantiateOption values
	// satisfying any import by name that the spec suite may require.
	// Seeded by buildSpecHost; extended whenever a test instantiates a
	// named component that subsequent components might import.
	importOpts []core.InstantiateOption

	// namedInstances maps a wast-declared name to its instantiated
	// core.ComponentInstance. Used by assert_return / assert_trap etc. to
	// locate the instance whose exported function to invoke.
	namedInstances map[string]*core.ComponentInstance

	// lastComponent is the most recently loaded anonymous module that
	// has not yet been instantiated. Parser-only files (e.g. wasm-tools/)
	// frequently declare modules whose imports cannot be satisfied, so
	// instantiation is deferred until a runtime assertion needs it.
	lastComponent *core.Component

	// lastInstance is the most recently instantiated module, named or
	// anonymous. Cleared whenever a new module directive is processed.
	lastInstance *core.ComponentInstance
}

func TestSpecConformance(t *testing.T) {
	layout, err := spectest.Discover()
	if err != nil {
		t.Fatalf("discover testdata layout: %v", err)
	}
	manifest, err := spectest.LoadSkipManifest(layout.ManifestPath)
	if err != nil {
		t.Fatalf("load skip manifest: %v", err)
	}
	files, err := layout.Plan(manifest)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := spectest.CheckFixturesExist(files); err != nil {
		t.Fatal(err)
	}

	var ranFiles, ranAsserts, skippedFiles, skippedAsserts atomic.Int64
	for _, f := range files {
		runName := fmt.Sprintf("%s/%s", f.Source, strings.TrimSuffix(f.Rel, ".wast"))
		t.Run(runName, func(t *testing.T) {
			if f.SkipReason != "" {
				skippedFiles.Add(1)
				t.Skip(f.SkipReason)
				return
			}
			ranFiles.Add(1)

			data, err := os.ReadFile(f.FixtureJSON)
			if err != nil {
				t.Fatal(err)
			}
			var spec specJSON
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			engine := core.NewEngine(ctx)
			defer engine.Close(ctx)

			hostOpts, err := buildSpecHost(ctx, engine)
			if err != nil {
				t.Fatalf("build host harness: %v", err)
			}

			state := &specTestState{
				t:              t,
				ctx:            ctx,
				engine:         engine,
				definitions:    make(map[string]*core.Component),
				importOpts:     hostOpts,
				namedInstances: make(map[string]*core.ComponentInstance),
			}

			for i, cmd := range spec.Commands {
				cmd := cmd
				testName := fmt.Sprintf("%s-%d-line%d", cmd.Type, i, cmd.Line)
				t.Run(testName, func(t *testing.T) {
					if f.Source == spectest.SourceSpec {
						if reason := manifest.AssertionSkip(f.Rel, cmd.Line); reason != "" {
							skippedAsserts.Add(1)
							t.Skip(reason)
							return
						}
					}
					ranAsserts.Add(1)
					state.dispatch(t, f.FixtureWasmDir, cmd)
				})
			}
		})
	}
	t.Logf("core spec coverage: %d files run, %d assertions ran, %d files whole-skipped, %d assertions skipped",
		ranFiles.Load(), ranAsserts.Load(), skippedFiles.Load(), skippedAsserts.Load())
}

func (s *specTestState) dispatch(t *testing.T, wasmDir string, cmd specCommand) {
	switch cmd.Type {
	case "module":
		s.handleModule(t, wasmDir, cmd)
	case "module_definition":
		s.handleModuleDefinition(t, wasmDir, cmd)
	case "module_instance":
		s.handleModuleInstance(t, cmd)
	case "assert_return":
		s.handleAssertReturn(t, cmd)
	case "assert_trap":
		s.handleAssertTrap(t, cmd)
	case "assert_invalid":
		s.handleAssertInvalid(t, wasmDir, cmd)
	case "assert_malformed":
		s.handleAssertMalformed(t, wasmDir, cmd)
	case "assert_unlinkable":
		s.handleAssertUnlinkable(t, wasmDir, cmd)
	case "assert_uninstantiable":
		s.handleAssertUninstantiable(t, wasmDir, cmd)
	default:
		t.Fatalf("unsupported command type: %s", cmd.Type)
	}
}

func (s *specTestState) handleModule(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" {
		t.Skip("no filename")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatalf("line %d: read wasm: %v", cmd.Line, err)
	}

	comp, err := s.engine.LoadComponent(s.ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("line %d: LoadComponent failed: %v", cmd.Line, err)
	}

	s.lastComponent = comp
	s.lastInstance = nil

	// Named modules are instantiated immediately so they can be wired into
	// importOpts for any subsequent component that imports them. Anonymous
	// modules are instantiated lazily on first runtime use, so parser-only
	// suites (wasm-tools/) don't fail on unsatisfiable imports.
	if cmd.Name != "" {
		inst, err := comp.Instantiate(s.ctx, s.importOpts...)
		if err != nil {
			t.Fatalf("line %d: Instantiate failed: %v", cmd.Line, err)
		}
		s.namedInstances[cmd.Name] = inst
		s.importOpts = append(s.importOpts, core.WithInstanceImport(cmd.Name, inst))
		s.lastInstance = inst
	}
}

// instantiateLast returns the lazy instance for the most recent anonymous
// module directive, instantiating it on first use. Returns nil if no module
// has been loaded yet.
func (s *specTestState) instantiateLast() (*core.ComponentInstance, error) {
	if s.lastInstance != nil {
		return s.lastInstance, nil
	}
	if s.lastComponent == nil {
		return nil, nil
	}
	inst, err := s.lastComponent.Instantiate(s.ctx, s.importOpts...)
	if err != nil {
		return nil, err
	}
	s.lastInstance = inst
	return inst, nil
}

func (s *specTestState) handleModuleDefinition(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" {
		t.Skip("no filename")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatalf("line %d: read wasm: %v", cmd.Line, err)
	}

	comp, err := s.engine.LoadComponent(s.ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("line %d: LoadComponent failed: %v", cmd.Line, err)
	}

	// Nameless module_definitions are parser-only declarations (common in
	// wasm-tools/) — the .wast file just wants to verify the binary
	// parses+validates. Named module_definitions are stored for later
	// module_instance directives.
	if cmd.Name != "" {
		s.definitions[cmd.Name] = comp
	}
}

func (s *specTestState) handleModuleInstance(t *testing.T, cmd specCommand) {
	t.Helper()

	comp, ok := s.definitions[cmd.Module]
	if !ok {
		t.Fatalf("line %d: module_instance references undefined module %q", cmd.Line, cmd.Module)
		return
	}

	opts := s.importOpts

	inst, err := comp.Instantiate(s.ctx, opts...)
	if err != nil {
		t.Fatalf("line %d: Instantiate module %q as %q failed: %v", cmd.Line, cmd.Module, cmd.Instance, err)
	}

	s.lastInstance = inst
	if cmd.Instance != "" {
		s.namedInstances[cmd.Instance] = inst
		s.importOpts = append(s.importOpts, core.WithInstanceImport(cmd.Instance, inst))
	}
}

func (s *specTestState) handleAssertReturn(t *testing.T, cmd specCommand) {
	t.Helper()

	if cmd.Action == nil {
		t.Fatalf("line %d: assert_return without action", cmd.Line)
		return
	}

	if cmd.Action.Type != "invoke" {
		t.Fatalf("line %d: unsupported action type %q", cmd.Line, cmd.Action.Type)
		return
	}

	var inst *core.ComponentInstance
	if cmd.Action.Module != "" {
		var ok bool
		inst, ok = s.namedInstances[cmd.Action.Module]
		if !ok {
			t.Fatalf("line %d: assert_return references unknown module %q", cmd.Line, cmd.Action.Module)
			return
		}
	} else {
		var err error
		inst, err = s.instantiateLast()
		if err != nil {
			t.Fatalf("line %d: lazy Instantiate failed: %v", cmd.Line, err)
			return
		}
	}

	if inst == nil {
		t.Fatalf("line %d: assert_return but no component instance available (prior instantiation skipped)", cmd.Line)
		return
	}

	fn := inst.ExportedFunc(cmd.Action.Field)
	if fn == nil {
		t.Fatalf("line %d: exported function %q not found", cmd.Line, cmd.Action.Field)
		return
	}

	args, err := specValuesToArgs(cmd.Action.Args)
	if err != nil {
		t.Fatalf("line %d: unsupported arg type: %v", cmd.Line, err)
		return
	}

	results, err := fn.Call(s.ctx, args...)
	if err != nil {
		t.Fatalf("line %d: call %q failed (possibly unimplemented feature): %v", cmd.Line, cmd.Action.Field, err)
		return
	}

	if len(cmd.Expected) == 0 {
		return
	}

	expected, err := specValuesToExpected(cmd.Expected)
	if err != nil {
		t.Fatalf("line %d: unsupported expected type: %v", cmd.Line, err)
		return
	}

	if len(results) != len(expected) {
		t.Fatalf("line %d: %q returned %d values, expected %d (result count mismatch)", cmd.Line, cmd.Action.Field, len(results), len(expected))
		return
	}

	for i, exp := range expected {
		if !valEqual(results[i], exp) {
			t.Fatalf("line %d: %q result[%d]: got %v (%T), want %v (%T)",
				cmd.Line, cmd.Action.Field, i, results[i], results[i], exp, exp)
		}
	}
}

func (s *specTestState) handleAssertTrap(t *testing.T, cmd specCommand) {
	t.Helper()

	if cmd.Action == nil {
		t.Fatalf("line %d: assert_trap without action", cmd.Line)
		return
	}

	if cmd.Action.Type != "invoke" {
		t.Fatalf("line %d: unsupported action type %q", cmd.Line, cmd.Action.Type)
		return
	}

	var inst *core.ComponentInstance
	if cmd.Action.Module != "" {
		var ok bool
		inst, ok = s.namedInstances[cmd.Action.Module]
		if !ok {
			t.Fatalf("line %d: assert_trap references unknown module %q", cmd.Line, cmd.Action.Module)
			return
		}
	} else {
		var err error
		inst, err = s.instantiateLast()
		if err != nil {
			t.Fatalf("line %d: lazy Instantiate failed: %v", cmd.Line, err)
			return
		}
	}

	if inst == nil {
		t.Fatalf("line %d: assert_trap but no component instance available (prior instantiation skipped)", cmd.Line)
		return
	}

	fn := inst.ExportedFunc(cmd.Action.Field)
	if fn == nil {
		t.Fatalf("line %d: exported function %q not found", cmd.Line, cmd.Action.Field)
		return
	}

	args, err := specValuesToArgs(cmd.Action.Args)
	if err != nil {
		t.Fatalf("line %d: unsupported arg type: %v", cmd.Line, err)
		return
	}

	_, err = fn.Call(s.ctx, args...)
	if err == nil {
		t.Fatalf("line %d: expected trap calling %q, but succeeded (resource semantics not yet complete)", cmd.Line, cmd.Action.Field)
	}
	if cmd.Text != "" && !strings.Contains(err.Error(), cmd.Text) {
		t.Fatalf("line %d: trap error %q does not contain %q", cmd.Line, err.Error(), cmd.Text)
	}
}

func (s *specTestState) handleAssertInvalid(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" || cmd.ModuleType == "text" {
		t.Skip("text-only assert_invalid")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatal(err)
	}

	var validationErr error
	vp := wasmparser.NewValidatingParser(bytes.NewReader(data), wasmparser.AllFeatures())
	for {
		_, err := vp.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			validationErr = err
			break
		}
	}

	if validationErr == nil {
		_, loadErr := s.engine.LoadComponent(s.ctx, bytes.NewReader(data))
		if loadErr != nil {
			validationErr = loadErr
		}
	}

	if validationErr == nil {
		t.Fatalf("line %d: expected validation error containing %q, but validation passed", cmd.Line, cmd.Text)
	}

	if cmd.Text != "" && !strings.Contains(validationErr.Error(), cmd.Text) {
		t.Fatalf("line %d: validation error %q does not contain expected %q", cmd.Line, validationErr.Error(), cmd.Text)
	}
}

func (s *specTestState) handleAssertMalformed(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" || cmd.ModuleType == "text" {
		t.Skip("text-only assert_malformed")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatal(err)
	}

	var parseErr error
	for _, err := range wasmparser.ParseAll(bytes.NewReader(data)) {
		if err != nil {
			parseErr = err
			break
		}
	}
	if parseErr == nil {
		vp := wasmparser.NewValidatingParser(bytes.NewReader(data), wasmparser.AllFeatures())
		for {
			_, err := vp.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				parseErr = err
				break
			}
		}
	}
	if parseErr == nil {
		t.Fatalf("line %d: expected parse error containing %q, but succeeded", cmd.Line, cmd.Text)
	}
	if cmd.Text != "" && !strings.Contains(parseErr.Error(), cmd.Text) {
		t.Fatalf("line %d: parse error %q does not contain %q", cmd.Line, parseErr.Error(), cmd.Text)
	}
}

func (s *specTestState) handleAssertUnlinkable(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" {
		t.Skip("no filename")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatal(err)
	}

	comp, err := s.engine.LoadComponent(s.ctx, bytes.NewReader(data))
	if err != nil {
		return
	}

	opts := s.importOpts

	_, instErr := comp.Instantiate(s.ctx, opts...)
	if instErr == nil {
		t.Fatalf("line %d: expected unlinkable error containing %q, but instantiation succeeded", cmd.Line, cmd.Text)
	}

	if cmd.Text != "" && !strings.Contains(instErr.Error(), cmd.Text) {
		t.Fatalf("line %d: unlinkable error %q does not contain expected %q", cmd.Line, instErr.Error(), cmd.Text)
	}
}

func (s *specTestState) handleAssertUninstantiable(t *testing.T, wasmDir string, cmd specCommand) {
	t.Helper()

	if cmd.Filename == "" {
		t.Skip("no filename")
	}

	data, err := os.ReadFile(filepath.Join(wasmDir, cmd.Filename))
	if err != nil {
		t.Fatal(err)
	}

	comp, err := s.engine.LoadComponent(s.ctx, bytes.NewReader(data))
	if err != nil {
		return
	}

	opts := s.importOpts

	_, instErr := comp.Instantiate(s.ctx, opts...)
	if instErr == nil {
		t.Fatalf("line %d: expected uninstantiable error containing %q, but succeeded", cmd.Line, cmd.Text)
	}

	if cmd.Text != "" && !strings.Contains(instErr.Error(), cmd.Text) {
		t.Fatalf("line %d: uninstantiable error %q does not contain expected %q", cmd.Line, instErr.Error(), cmd.Text)
	}
}

// --- Value conversion helpers ---

func specValuesToArgs(svs []specValue) ([]core.Val, error) {
	vals := make([]core.Val, 0, len(svs))
	for _, sv := range svs {
		v, err := specValueToVal(sv)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, nil
}

func specValuesToExpected(svs []specValue) ([]core.Val, error) {
	return specValuesToArgs(svs)
}

func specValueToVal(sv specValue) (core.Val, error) {
	switch sv.Type {
	case "bool":
		var b bool
		if err := json.Unmarshal(sv.Value, &b); err != nil {
			return nil, fmt.Errorf("parse bool: %w", err)
		}
		return core.ValBool(b), nil

	case "u8":
		n, err := parseUintFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse u8: %w", err)
		}
		return core.ValU8(n), nil

	case "s8":
		n, err := parseIntFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse s8: %w", err)
		}
		return core.ValS8(n), nil

	case "u16":
		n, err := parseUintFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse u16: %w", err)
		}
		return core.ValU16(n), nil

	case "s16":
		n, err := parseIntFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse s16: %w", err)
		}
		return core.ValS16(n), nil

	case "u32":
		n, err := parseUintFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse u32: %w", err)
		}
		return core.ValU32(n), nil

	case "s32":
		n, err := parseIntFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse s32: %w", err)
		}
		return core.ValS32(n), nil

	case "u64":
		n, err := parseUintFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse u64: %w", err)
		}
		return core.ValU64(n), nil

	case "s64":
		n, err := parseIntFromJSON(sv.Value)
		if err != nil {
			return nil, fmt.Errorf("parse s64: %w", err)
		}
		return core.ValS64(n), nil

	case "f32":
		var s string
		if err := json.Unmarshal(sv.Value, &s); err != nil {
			return nil, fmt.Errorf("parse f32 string: %w", err)
		}
		bits, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parse f32 bits: %w", err)
		}
		return core.ValF32(math.Float32frombits(uint32(bits))), nil

	case "f64":
		var s string
		if err := json.Unmarshal(sv.Value, &s); err != nil {
			return nil, fmt.Errorf("parse f64 string: %w", err)
		}
		bits, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse f64 bits: %w", err)
		}
		return core.ValF64(math.Float64frombits(bits)), nil

	case "char":
		var s string
		if err := json.Unmarshal(sv.Value, &s); err != nil {
			return nil, fmt.Errorf("parse char: %w", err)
		}
		runes := []rune(s)
		if len(runes) == 0 {
			return core.ValChar(0), nil
		}
		return core.ValChar(runes[0]), nil

	case "string":
		var s string
		if err := json.Unmarshal(sv.Value, &s); err != nil {
			return nil, fmt.Errorf("parse string: %w", err)
		}
		return core.ValString(s), nil

	case "list":
		var raw json.RawMessage
		if err := json.Unmarshal(sv.Value, &raw); err != nil {
			return nil, fmt.Errorf("parse list: %w", err)
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("parse list array: %w", err)
		}
		if len(arr) == 0 {
			return &core.ValList{}, nil
		}
		return nil, fmt.Errorf("non-empty list values not yet supported")

	default:
		return nil, fmt.Errorf("unsupported value type %q", sv.Type)
	}
}

func parseUintFromJSON(raw json.RawMessage) (uint64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseUint(s, 10, 64)
	}
	var n uint64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("cannot parse %q as uint", string(raw))
	}
	return n, nil
}

func parseIntFromJSON(raw json.RawMessage) (int64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strconv.ParseInt(s, 10, 64)
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("cannot parse %q as int", string(raw))
	}
	return n, nil
}

func valEqual(a, b core.Val) bool {
	switch av := a.(type) {
	case core.ValBool:
		bv, ok := b.(core.ValBool)
		return ok && av == bv
	case core.ValU8:
		bv, ok := b.(core.ValU8)
		return ok && av == bv
	case core.ValS8:
		bv, ok := b.(core.ValS8)
		return ok && av == bv
	case core.ValU16:
		bv, ok := b.(core.ValU16)
		return ok && av == bv
	case core.ValS16:
		bv, ok := b.(core.ValS16)
		return ok && av == bv
	case core.ValU32:
		bv, ok := b.(core.ValU32)
		return ok && av == bv
	case core.ValS32:
		bv, ok := b.(core.ValS32)
		return ok && av == bv
	case core.ValU64:
		bv, ok := b.(core.ValU64)
		return ok && av == bv
	case core.ValS64:
		bv, ok := b.(core.ValS64)
		return ok && av == bv
	case core.ValF32:
		bv, ok := b.(core.ValF32)
		if !ok {
			return false
		}
		if math.IsNaN(float64(av)) && math.IsNaN(float64(bv)) {
			return true
		}
		return av == bv
	case core.ValF64:
		bv, ok := b.(core.ValF64)
		if !ok {
			return false
		}
		if math.IsNaN(float64(av)) && math.IsNaN(float64(bv)) {
			return true
		}
		return av == bv
	case core.ValChar:
		bv, ok := b.(core.ValChar)
		return ok && av == bv
	case core.ValString:
		bv, ok := b.(core.ValString)
		return ok && av == bv
	default:
		return false
	}
}
