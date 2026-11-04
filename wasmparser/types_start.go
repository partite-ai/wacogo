package wasmparser

// ComponentStartFunction represents a start function in a component.
type ComponentStartFunction struct {
	FuncIndex uint32
	Args      []uint32
	Results   uint32
}

func (s *ComponentStartFunction) unmarshalBinary(r *BinaryReader) error {
	funcIdx, err := r.ReadU32()
	if err != nil {
		return err
	}
	s.FuncIndex = funcIdx

	argCount, err := r.ReadU32()
	if err != nil {
		return err
	}
	if argCount > r.limits.StartArgs {
		return errfAt(r.Offset(), "start function arg count %d exceeds maximum %d", argCount, r.limits.StartArgs)
	}
	s.Args = make([]uint32, 0, argCount)
	for range argCount {
		arg, err := r.ReadU32()
		if err != nil {
			return err
		}
		s.Args = append(s.Args, arg)
	}

	results, err := r.ReadU32()
	if err != nil {
		return err
	}
	s.Results = results
	return nil
}
