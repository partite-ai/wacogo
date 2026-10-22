package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

func TestCoreModuleTablesExcludeBorrowedImports(t *testing.T) {
	data := buildComponentBytes(t, `(module (import "host" "table" (table 2 3 funcref)) (table 1 4 funcref) (table 0 externref))`)
	tables, err := CoreModuleTables(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 || tables[0].Min != 1 || !tables[0].Max.Valid || tables[0].Max.Value != 4 || tables[1].Min != 0 || tables[1].Max.Valid {
		t.Fatalf("defined tables = %+v", tables)
	}
	if _, err := CoreModuleTables(buildComponentBytes(t, `(component)`)); err == nil {
		t.Fatal("component accepted as a core module")
	}
	if _, err := CoreModuleTables([]byte("invalid")); err == nil {
		t.Fatal("invalid module accepted")
	}
}

type tableAdmission struct {
	limit, live, calls, releases int
}

var errTableAdmission = errors.New("table admission refused")

func (a *tableAdmission) allocate(tables []CoreTable) (func(), error) {
	a.calls++
	if len(tables) != 1 || !tables[0].Max.Valid || a.live == a.limit {
		return nil, errTableAdmission
	}
	a.live++
	return func() { a.live--; a.releases++ }, nil
}

const repeatedTableComponent = `(component
  (core module $owner (table (export "table") 1 2 funcref))
  (core instance $one (instantiate $owner))
  (core instance (instantiate $owner))
  (core module $borrower (import "owner" "table" (table 1 2 funcref)))
  (core instance (instantiate $borrower (with "owner" (instance $one))))
  (component $child
    (core module $owner (table 1 2 funcref))
    (core instance (instantiate $owner)))
  (instance (instantiate $child))
  (instance (instantiate $child)))`

func TestCoreTableAdmissionCountsRepeatedAndNestedInstances(t *testing.T) {
	for _, limit := range []int{3, 4} {
		t.Run(map[int]string{3: "refused", 4: "accepted"}[limit], func(t *testing.T) {
			admission := &tableAdmission{limit: limit}
			ctx := WithCoreTableAllocator(t.Context(), admission.allocate)
			engine := NewEngine(ctx)
			t.Cleanup(func() { _ = engine.Close(context.WithoutCancel(ctx)) })
			component, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, repeatedTableComponent)))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if limit == 3 {
				if !errors.Is(err, errTableAdmission) || instance != nil || admission.live != 0 || admission.releases != 3 {
					t.Fatalf("failed nested admission leaked: instance=%v err=%v state=%+v", instance, err, admission)
				}
			} else {
				if err != nil || admission.live != 4 {
					t.Fatalf("accepted allocation: err=%v state=%+v", err, admission)
				}
				for range 2 {
					if err := instance.Close(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if admission.live != 0 || admission.releases != 4 {
					t.Fatalf("reservation not released once: %+v", admission)
				}
			}
			if admission.calls != 4 {
				t.Fatalf("borrowed table charged or nested instance missed: %+v", admission)
			}
		})
	}
}

func TestCoreTableAdmissionRollsBackStartTrap(t *testing.T) {
	admission := &tableAdmission{limit: 1}
	ctx := WithCoreTableAllocator(t.Context(), admission.allocate)
	engine := NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(context.WithoutCancel(ctx)) })
	component, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, `(component
  (core module $m (table 0 2 funcref) (func $start unreachable) (start $start))
  (core instance (instantiate $m)))`)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := component.Instantiate(ctx); err == nil || admission.live != 0 || admission.releases != 1 {
		t.Fatalf("start failure leaked table reservation: err=%v state=%+v", err, admission)
	}
}

func TestCoreTableAdmissionRetainsBoundedGrowth(t *testing.T) {
	admission := &tableAdmission{limit: 1}
	ctx := WithCoreTableAllocator(t.Context(), admission.allocate)
	engine := NewEngine(ctx)
	t.Cleanup(func() { _ = engine.Close(context.WithoutCancel(ctx)) })
	component, err := engine.LoadComponent(ctx, bytes.NewReader(buildComponentBytes(t, `(component
  (core module $m (table 0 2 funcref)
    (func $start
      ref.null func i32.const 2 table.grow i32.const 0 i32.ne if unreachable end
      ref.null func i32.const 1 table.grow i32.const -1 i32.ne if unreachable end)
    (start $start))
  (core instance (instantiate $m)))`)))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := component.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Close(ctx); err != nil || admission.live != 0 || admission.releases != 1 {
		t.Fatalf("bounded growth changed allocation ownership: err=%v state=%+v", err, admission)
	}
}

func TestCoreTableAdmissionEngineShutdown(t *testing.T) {
	data := buildComponentBytes(t, repeatedTableComponent)
	for _, componentFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "engine first", true: "component first"}[componentFirst], func(t *testing.T) {
			admission := &tableAdmission{limit: 4}
			var instance *ComponentInstance
			ctx := WithCoreTableAllocator(t.Context(), func(tables []CoreTable) (func(), error) {
				release, err := admission.allocate(tables)
				if err != nil {
					return release, err
				}
				return func() {
					release()
					if instance != nil {
						if err := instance.Close(context.Background()); err != nil {
							t.Error(err)
						}
					}
				}, nil
			})
			engine := NewEngine(ctx)
			t.Cleanup(func() { _ = engine.Close(context.WithoutCancel(ctx)) })
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err = component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if componentFirst {
				if err := instance.Close(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := engine.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if admission.live != 0 || admission.releases != 4 {
				t.Fatalf("shutdown leaked quota: %+v", admission)
			}
			if err := instance.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if admission.releases != 4 {
				t.Fatalf("duplicate refund: %+v", admission)
			}
			if len(engine.tableModules) != 0 {
				t.Fatal("closed modules retained")
			}
		})
	}
}

func TestCoreTableAdmissionShutdownDuringAdmission(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref))
      (core instance (instantiate $m)))`)
	for _, reentrant := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent", true: "reentrant"}[reentrant], func(t *testing.T) {
			engine := NewEngine(t.Context())
			defer engine.Close(context.Background())
			entered, resume := make(chan struct{}), make(chan struct{})
			releases := 0
			ctx := WithCoreTableAllocator(t.Context(), func([]CoreTable) (func(), error) {
				if reentrant {
					if err := engine.Close(context.Background()); err != nil {
						t.Error(err)
					}
				} else {
					close(entered)
					<-resume
				}
				return func() { releases++ }, nil
			})
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instantiate := func() error {
				instance, err := component.Instantiate(ctx)
				if instance != nil {
					t.Error("instantiation succeeded after shutdown")
				}
				return err
			}
			if reentrant {
				if err := instantiate(); err == nil {
					t.Fatal("missing shutdown error")
				}
			} else {
				instantiated := make(chan error, 1)
				go func() { instantiated <- instantiate() }()
				<-entered
				if err := engine.Close(ctx); err != nil {
					t.Error(err)
				}
				if releases != 0 {
					t.Error("refunded before allocator returned")
				}
				close(resume)
				if err := <-instantiated; err == nil {
					t.Fatal("missing shutdown error")
				}
			}
			if releases != 1 {
				t.Fatalf("refunds = %d", releases)
			}
		})
	}
}

func TestCoreTableAdmissionShutdownFromCloseNotifier(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref) (memory 1))
      (core instance (instantiate $m)))`)
	for _, componentFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime teardown", true: "explicit teardown"}[componentFirst], func(t *testing.T) {
			allocator := &teardownAllocator{}
			ctx := experimental.WithMemoryAllocator(t.Context(), allocator)
			engine := NewEngine(ctx)
			defer engine.Close(context.Background())
			releases, notifications := 0, 0
			ctx = WithCoreTableAllocator(ctx, func([]CoreTable) (func(), error) {
				return func() {
					allocator.check(t, 1, 0, []uint64{65536})
					releases++
				}, nil
			})
			ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(context.Context, uint32) {
				notifications++
				if err := engine.Close(context.Background()); err != nil {
					t.Error(err)
				}
				if releases != 0 {
					t.Error("refunded inside pre-teardown notifier")
				}
			}))
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if componentFirst {
				if err := instance.Close(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := engine.Close(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if releases != 1 || notifications != 1 {
				t.Fatalf("refunds=%d notifications=%d", releases, notifications)
			}
			if err := instance.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if releases != 1 {
				t.Fatalf("duplicate refund: %d", releases)
			}
		})
	}
}

func TestCoreTableAdmissionConcurrentTeardown(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref) (memory 1))
      (core instance (instantiate $m)))`)
	for _, runtimeFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit first", true: "runtime first"}[runtimeFirst], func(t *testing.T) {
			allocator := &teardownAllocator{}
			ctx := experimental.WithMemoryAllocator(t.Context(), allocator)
			engine := NewEngine(ctx)
			defer engine.Close(context.Background())
			entered, resume := make(chan struct{}), make(chan struct{})
			releases := 0
			ctx = WithCoreTableAllocator(ctx, func([]CoreTable) (func(), error) {
				return func() {
					allocator.check(t, 1, 0, []uint64{65536})
					releases++
				}, nil
			})
			ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(context.Context, uint32) {
				close(entered)
				<-resume
			}))
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			first, second := instance.Close, engine.Close
			if runtimeFirst {
				first, second = second, first
			}
			finished := make(chan error, 1)
			go func() { finished <- first(ctx) }()
			<-entered
			if err := second(ctx); err != nil {
				t.Error(err)
			}
			if releases != 0 {
				t.Error("refunded before teardown completed")
			}
			close(resume)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if releases != 1 {
				t.Fatalf("refunds = %d", releases)
			}
		})
	}
}

func TestCoreTableAdmissionCancellationNotifierShutdown(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref) (memory 1) (func (export "run")))
      (core instance $i (instantiate $m))
      (func (export "run") (canon lift (core func $i "run"))))`)
	for _, call := range []string{"Call", "CallRaw", "CloseDestructor"} {
		t.Run(call, func(t *testing.T) {
			allocator := &teardownAllocator{}
			ctx := experimental.WithMemoryAllocator(t.Context(), allocator)
			engine := NewEngine(ctx, WithRuntimeConfig(wazero.NewRuntimeConfig().WithCloseOnContextDone(true)))
			defer engine.Close(context.Background())
			releases, notifications := 0, 0
			ctx = WithCoreTableAllocator(ctx, func([]CoreTable) (func(), error) {
				return func() {
					allocator.mu.Lock()
					if allocator.live != 0 {
						t.Errorf("refund before memory free: live=%d", allocator.live)
					}
					allocator.mu.Unlock()
					releases++
				}, nil
			})
			ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(context.Context, uint32) {
				notifications++
				if err := engine.Close(context.Background()); err != nil {
					t.Error(err)
				}
				if releases != 0 {
					t.Error("refunded inside cancellation notifier")
				}
			}))
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			fn := instance.ExportedFunc("run")
			if call == "CloseDestructor" {
				for _, owned := range instance.owned {
					if m, ok := owned.(*accountedModule); ok {
						instance.resources.IssueOwn(&TypeResource{instance: instance, dtor: func(ctx context.Context, _ uint32) error {
							_, err := m.module.ExportedFunction("run").Call(ctx)
							return err
						}}, 1)
					}
				}
				err = instance.Close(canceled)
			} else if call == "CallRaw" {
				err = fn.CallRaw(canceled, nil, func(*CallContext, *CallContext, []uint64) {}, func(*CallContext, *CallContext, []uint64) {})
			} else {
				_, err = fn.Call(canceled)
			}
			if err == nil {
				t.Fatal("canceled call succeeded")
			}
			if releases != 1 || notifications != 1 {
				t.Fatalf("refunds=%d notifications=%d", releases, notifications)
			}
			if err := instance.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if releases != 1 {
				t.Fatalf("duplicate refund: %d", releases)
			}
		})
	}
}

func TestCoreTableAdmissionRefundPanics(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref) (memory 1) (func (export "run")))
      (core instance $a (instantiate $m))
      (core instance (instantiate $m))
      (component $child
        (core module $m (table 0 2 funcref))
        (core instance (instantiate $m))
        (core instance (instantiate $m)))
      (instance (instantiate $child))
      (func (export "run") (canon lift (core func $a "run"))))`)
	for _, operation := range []string{"engine", "component", "Call", "CallRaw", "CallRawPanic", "RunInComponent", "ExitPanic", "Drop"} {
		t.Run(operation, func(t *testing.T) {
			allocator := &teardownAllocator{}
			ctx := experimental.WithMemoryAllocator(t.Context(), allocator)
			engine := NewEngine(ctx, WithRuntimeConfig(wazero.NewRuntimeConfig().WithCloseOnContextDone(true)))
			defer engine.Close(context.Background())
			var failures []error
			var releases []int
			var notifierErr error
			callPanic := errors.New("result decoder panic")
			ctx = WithCoreTableAllocator(ctx, func([]CoreTable) (func(), error) {
				id := len(failures)
				failure := fmt.Errorf("release %d", id)
				failures = append(failures, failure)
				releases = append(releases, 0)
				return func() {
					allocator.mu.Lock()
					live := allocator.live
					allocator.mu.Unlock()
					if id < 2 && live != 0 {
						t.Errorf("refund before memory free: live=%d", live)
					}
					releases[id]++
					panic(failure)
				}, nil
			})
			if operation == "Call" || operation == "CallRaw" {
				ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(context.Context, uint32) {
					notifierErr = errors.Join(notifierErr, engine.Close(context.Background()))
				}))
			}
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var failure error
			func() {
				defer func() {
					if p := recover(); p != nil {
						var ok bool
						failure, ok = p.(error)
						if !ok {
							t.Errorf("unexpected panic: %v", p)
						}
					}
				}()
				switch operation {
				case "engine":
					failure = engine.Close(ctx)
				case "component":
					failure = instance.Close(ctx)
				case "RunInComponent":
					failure = instance.RunInComponent(ctx, func() error {
						return errors.Join(callPanic, engine.Close(ctx))
					})
				case "Drop":
					table := NewResourceTable(&ComponentInstance{})
					handle := table.IssueOwn(&TypeResource{instance: instance, dtor: func(context.Context, uint32) error {
						return errors.Join(callPanic, engine.Close(ctx))
					}}, 1)
					failure = handle.Drop(ctx)
				case "ExitPanic":
					if err := instance.Enter(ctx); err != nil {
						t.Fatal(err)
					}
					defer instance.Exit(ctx)
					notifierErr = engine.Close(ctx)
					panic(callPanic)
				case "CallRawPanic":
					failure = instance.ExportedFunc("run").CallRaw(ctx, nil,
						func(*CallContext, *CallContext, []uint64) {},
						func(*CallContext, *CallContext, []uint64) {
							notifierErr = engine.Close(ctx)
							panic(callPanic)
						})
				default:
					canceled, cancel := context.WithCancel(ctx)
					cancel()
					fn := instance.ExportedFunc("run")
					if operation == "Call" {
						_, failure = fn.Call(canceled)
					} else {
						failure = fn.CallRaw(canceled, nil, func(*CallContext, *CallContext, []uint64) {}, func(*CallContext, *CallContext, []uint64) {})
					}
				}
			}()
			failure = errors.Join(failure, notifierErr)
			if (operation == "CallRawPanic" || operation == "RunInComponent" || operation == "ExitPanic" || operation == "Drop") && !errors.Is(failure, callPanic) {
				t.Errorf("lost original operation failure: %v", failure)
			}
			if (operation == "Call" || operation == "CallRaw") && !errors.Is(failure, context.Canceled) {
				t.Errorf("lost original cancellation: %v", failure)
			}
			for id, count := range releases {
				if count != 1 {
					t.Errorf("release %d called %d times, want 1", id, count)
				}
				if !errors.Is(failure, failures[id]) {
					t.Errorf("lost release %d panic: %v", id, failure)
				}
			}
			if len(engine.tableModules) != 0 {
				t.Errorf("retained %d reservations", len(engine.tableModules))
			}
			if err := instance.Close(ctx); err != nil {
				t.Error(err)
			}
			if err := engine.Close(ctx); err != nil {
				t.Error(err)
			}
			for id, count := range releases {
				if count != 1 {
					t.Errorf("release %d repeated: %d", id, count)
				}
			}
		})
	}
}

func TestCoreTableAdmissionRefundRollbackPanics(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref))
      (core instance (instantiate $m))
      (core instance (instantiate $m))
      (core module $trap (table 0 2 funcref) (func $start unreachable) (start $start))
      (core instance (instantiate $trap)))`)
	for _, refused := range []bool{false, true} {
		t.Run(map[bool]string{false: "start trap", true: "admission refused"}[refused], func(t *testing.T) {
			var failures []error
			var releases []int
			ctx := WithCoreTableAllocator(t.Context(), func([]CoreTable) (func(), error) {
				id := len(failures)
				failure := fmt.Errorf("rollback release %d", id)
				failures = append(failures, failure)
				releases = append(releases, 0)
				var err error
				if refused && id == 2 {
					err = errTableAdmission
				}
				return func() { releases[id]++; panic(failure) }, err
			})
			engine := NewEngine(ctx)
			defer engine.Close(ctx)
			component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := component.Instantiate(ctx)
			if instance != nil || err == nil {
				t.Fatalf("instantiation should fail: instance=%v err=%v", instance, err)
			}
			if refused && !errors.Is(err, errTableAdmission) {
				t.Errorf("lost admission error: %v", err)
			}
			if !refused && !strings.Contains(err.Error(), "unreachable") {
				t.Errorf("lost start trap: %v", err)
			}
			for id, failure := range failures {
				if releases[id] != 1 || !errors.Is(err, failure) {
					t.Errorf("release %d: count=%d err=%v", id, releases[id], err)
				}
			}
			if len(failures) != 3 || len(engine.tableModules) != 0 {
				t.Errorf("allocations=%d retained=%d", len(failures), len(engine.tableModules))
			}
		})
	}
}

type delayedTableModule struct {
	api.Module
	entered chan struct{}
	resume  chan struct{}
}

func (m *delayedTableModule) Close(ctx context.Context) error {
	close(m.entered)
	<-m.resume
	return m.Module.Close(ctx)
}

func TestCoreTableAdmissionRuntimeWinsCloseDuringUnwind(t *testing.T) {
	data := buildComponentBytes(t, `(component
      (core module $m (table 0 2 funcref) (memory 1))
      (core instance (instantiate $m)))`)
	allocator := &teardownAllocator{}
	ctx := experimental.WithMemoryAllocator(t.Context(), allocator)
	engine := NewEngine(ctx)
	defer engine.Close(context.Background())
	releases := 0
	ctx = WithCoreTableAllocator(ctx, func([]CoreTable) (func(), error) {
		return func() {
			allocator.mu.Lock()
			if allocator.live != 0 {
				t.Errorf("refund before memory free: live=%d", allocator.live)
			}
			allocator.mu.Unlock()
			releases++
		}, nil
	})
	notified, finishTeardown := make(chan struct{}), make(chan struct{})
	ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(context.Context, uint32) {
		close(notified)
		<-finishTeardown
	}))
	component, err := engine.LoadComponent(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := component.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var delayed *delayedTableModule
	for _, owned := range instance.owned {
		if m, ok := owned.(*accountedModule); ok {
			delayed = &delayedTableModule{Module: m.module, entered: make(chan struct{}), resume: make(chan struct{})}
			m.module = delayed
		}
	}
	if delayed == nil {
		t.Fatal("missing accounted module")
	}
	componentClosed := make(chan error, 1)
	go func() { componentClosed <- instance.Close(ctx) }()
	<-delayed.entered
	engineClosed := make(chan error, 1)
	go func() { engineClosed <- engine.Close(ctx) }()
	<-notified
	close(delayed.resume)
	if err := <-componentClosed; err != nil {
		t.Error(err)
	}
	if releases != 0 {
		t.Error("component unwind refunded unfinished runtime teardown")
	}
	close(finishTeardown)
	if err := <-engineClosed; err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("refunds=%d", releases)
	}
}
