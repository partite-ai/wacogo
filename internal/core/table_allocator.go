package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/partite-ai/wacogo/wasmparser"
	"github.com/tetratelabs/wazero/api"
)

// CoreTable describes a table defined by a core module. Imported tables are
// borrowed from their defining instance and do not allocate another table.
type CoreTable = wasmparser.CoreTableType

// CoreModuleTables reads defined table limits without instantiating the module.
// It does not replace the runtime's full module validation.
func CoreModuleTables(data []byte) ([]CoreTable, error) {
	p := wasmparser.NewParser(bytes.NewReader(data))
	payload, err := p.Next()
	if err != nil {
		return nil, err
	}
	version, ok := payload.(*wasmparser.VersionPayload)
	if !ok || version.Encoding != wasmparser.EncodingModule {
		return nil, errors.New("expected a core WebAssembly module")
	}
	var tables []CoreTable
	for {
		payload, err := p.Next()
		if err == io.EOF {
			return tables, nil
		}
		if err != nil {
			return nil, err
		}
		if section, ok := payload.(*wasmparser.ModuleTableSectionPayload); ok {
			for table, err := range section.Items() {
				if err != nil {
					return nil, err
				}
				tables = append(tables, table)
			}
		}
	}
}

type coreTableAllocatorKey struct{}

type coreTableAllocator func([]CoreTable) (func(), error)

// WithCoreTableAllocator installs admission accounting for each newly
// instantiated inline core module. The callback runs before table allocation,
// receives only defined tables, and may reject the instance. A returned release
// function runs once after module teardown (including engine shutdown), or if
// instantiation fails. Nested instances inherit the context; imported table
// aliases are not charged again.
// The callback must be safe for concurrent use and must not retain the slice.
// Release panics do not interrupt other refunds. Error-returning operations
// join cleanup failures with their primary error. Direct use of the void Exit
// method propagates cleanup failures as a panic after pending refunds finish.
// Error-valued panics remain discoverable through errors.Is.
// Admission runs only at instantiation, not on table.grow. A hard capacity
// budget must reserve each finite maximum upfront and reject or explicitly
// account for unbounded tables.
func WithCoreTableAllocator(ctx context.Context, allocate func([]CoreTable) (release func(), err error)) context.Context {
	return context.WithValue(ctx, coreTableAllocatorKey{}, coreTableAllocator(allocate))
}

type moduleClosePhase uint8

const (
	moduleLive moduleClosePhase = iota
	moduleClosing
	moduleClosed
)

// accountedModule shares teardown between component and engine ownership.
// A module's IsClosed flag alone does not guarantee teardown has finished.
type accountedModule struct {
	module   api.Module
	release  func()
	engine   *Engine
	owner    *ComponentInstance
	id       uint64
	refunded atomic.Bool
	phase    moduleClosePhase // guarded by engine.lifecycleMu
}

func (m *accountedModule) Close(ctx context.Context) error {
	e := m.engine
	e.lifecycleMu.Lock()
	if m.phase != moduleLive || e.closed.Load() {
		e.lifecycleMu.Unlock()
		return nil
	}
	m.phase = moduleClosing
	e.lifecycleMu.Unlock()

	err := m.module.Close(ctx)

	e.lifecycleMu.Lock()
	m.phase = moduleClosed
	// Runtime shutdown may have won the module's close CAS. Its teardown
	// must finish too before a concurrent explicit Close can refund quota.
	refund := (!e.closed.Load() || e.runtimeClosed) && m.owner.activeOperations == 0
	m.owner.refundPending = true
	e.lifecycleMu.Unlock()
	if refund {
		err = errors.Join(err, m.refund())
	}
	return err
}

func (m *accountedModule) refund() error {
	if !m.refunded.CompareAndSwap(false, true) {
		return nil
	}
	m.engine.lifecycleMu.Lock()
	delete(m.engine.tableModules, m.id)
	m.engine.lifecycleMu.Unlock()
	return releaseCoreTables(m.release)
}

// releaseCoreTables isolates user callbacks so a panic cannot interrupt other
// refunds or escape through a wazero close notifier before memory is freed.
func releaseCoreTables(release func()) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if cause, ok := p.(error); ok {
				err = fmt.Errorf("core table release: %w", cause)
			} else {
				err = fmt.Errorf("core table release: %v", p)
			}
		}
	}()
	release()
	return nil
}

// finishTableCleanup must be deferred directly. Error-returning boundaries
// supply result; void Exit boundaries use nil and propagate cleanup failures
// as panics. An already-unwinding panic retains its identity and cleanup errors.
func finishTableCleanup(result *error, cleanup func() error) {
	original := recover()
	cleanupErr := cleanup()
	if original != nil {
		if cleanupErr == nil {
			panic(original)
		}
		if result != nil {
			cleanupErr = errors.Join(*result, cleanupErr)
		}
		if cause, ok := original.(error); ok {
			panic(errors.Join(cause, cleanupErr))
		}
		panic(errors.Join(fmt.Errorf("component operation panic: %v", original), cleanupErr))
	}
	if result != nil {
		*result = errors.Join(*result, cleanupErr)
	} else if cleanupErr != nil {
		panic(cleanupErr)
	}
}

func (e *Engine) accountModule(owner *ComponentInstance, module api.Module, release func()) (*accountedModule, error) {
	e.lifecycleMu.Lock()
	e.nextTableModule++
	m := &accountedModule{module: module, release: release, engine: e, owner: owner, id: e.nextTableModule}
	if e.tableModules == nil {
		e.tableModules = make(map[uint64]*accountedModule)
	}
	e.tableModules[m.id] = m
	refund := e.runtimeClosed && owner.activeOperations == 0
	e.lifecycleMu.Unlock()
	if refund {
		return m, m.refund()
	}
	return m, nil
}

func reserveCoreTables(ctx context.Context, tables []CoreTable) (func(), error) {
	allocate, ok := ctx.Value(coreTableAllocatorKey{}).(coreTableAllocator)
	if !ok || allocate == nil || len(tables) == 0 {
		return nil, nil
	}
	// Each callback receives its own limits; compiled metadata stays immutable.
	copyOfTables := append([]CoreTable(nil), tables...)
	release, err := allocate(copyOfTables)
	if release != nil {
		release = sync.OnceFunc(release)
	}
	return release, err
}

// Operations include canonical calls and component teardown. Wazero can close
// a module directly during cancellation; its notifier runs before teardown.
// Keeping the owner's reservation charged until unwind covers that path too.
func (i *ComponentInstance) beginOperation() {
	if i.engine == nil {
		return
	}
	i.engine.lifecycleMu.Lock()
	i.activeOperations++
	i.engine.lifecycleMu.Unlock()
}

func (i *ComponentInstance) endOperation() error {
	e := i.engine
	if e == nil {
		return nil
	}
	e.lifecycleMu.Lock()
	i.activeOperations--
	var pending []*accountedModule
	if i.activeOperations == 0 && (i.refundPending || e.runtimeClosed) {
		i.refundPending = false
		for _, m := range e.tableModules {
			if m.owner == i && m.phase != moduleClosing && ((m.phase == moduleClosed && !e.closed.Load()) || e.runtimeClosed) {
				pending = append(pending, m)
			}
		}
	}
	e.lifecycleMu.Unlock()
	var errs []error
	for _, m := range pending {
		if err := m.refund(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
