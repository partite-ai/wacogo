package core

import (
	"context"
	"testing"
)

func TestPlanResolveTypePopulatesInstanceTable(t *testing.T) {
	c := &Component{
		typeResolvers: []typeResolver{primitiveResolver{t: TypeU32{}}},
	}
	inst := &ComponentInstance{component: c, types: make([]Type, 1)}
	state := &instantiationState{inst: inst}

	step := &planResolveType{typeID: 0}
	if err := step.execute(context.Background(), state); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := inst.types[0].(TypeU32); !ok {
		t.Fatalf("want TypeU32, got %T", inst.types[0])
	}
}
