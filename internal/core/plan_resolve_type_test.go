package core

import (
	"context"
	"testing"
)

func TestPlanResolveTypePopulatesTypeIndexSpace(t *testing.T) {
	c := &Component{
		typeResolvers: []typeResolver{primitiveResolver{t: TypeU32{}}},
	}
	inst := &ComponentInstance{component: c}
	state := &instantiationState{inst: inst, types: make([]Type, 1)}

	step := &planResolveType{typeID: 0}
	if err := step.execute(context.Background(), state); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := state.types[0].(TypeU32); !ok {
		t.Fatalf("want TypeU32, got %T", state.types[0])
	}
}
