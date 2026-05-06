// Package canon implements the WebAssembly Component Model canonical
// ABI via a visitor-driven closure-emission pipeline.
//
// Six concrete visitors (flatTransferVisitor, memTransferVisitor,
// valToFlatVisitor, valToMemVisitor, flatToValVisitor, memToValVisitor)
// walk a Type tree and emit transferPlanStep, gocallLowerStep, or
// gocallLiftStep closures. Two runners (runTransferPlan for
// component→component, runGocallPlan for Go→component) consume the
// compiled plans.
package canon
