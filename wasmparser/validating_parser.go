package wasmparser

import "io"

// ValidatingParser wraps a Parser and a Validator so that each payload is
// validated as it is parsed. Unlike ParseAll (which flattens nesting),
// ValidatingParser preserves hierarchical sub-parsers in
// ComponentSectionPayload and ModuleSectionPayload, allowing the caller to
// recurse into nested components while still getting validation.
//
// For nested components, the returned ComponentSectionPayload has its
// ValidatingParser field set to a child that shares this parser's Validator.
// If the caller does not fully traverse the sub-parser before calling Next()
// again, the remaining payloads are drained and validated automatically.
type ValidatingParser struct {
	parser  *Parser
	validator *Validator
	pending *ValidatingParser // sub-parser that may need draining
}

// NewValidatingParser creates a ValidatingParser that parses from r and
// validates with the given feature set. Each call allocates an independent
// *TypeArena; types produced by this parser do not share identity with
// types from other NewValidatingParser calls. Cross-parse handles (e.g.
// supplying one component's *InstanceType as an instance-import arg to
// another) are wired by the cross-arena resolver in CheckInstantiation.
func NewValidatingParser(r io.Reader, features FeatureSet) *ValidatingParser {
	return &ValidatingParser{
		parser:    NewParser(r),
		validator: NewValidator(features),
	}
}

// Next returns the next validated payload. It returns io.EOF when the
// parser is exhausted.
//
// For ComponentSectionPayload, the ValidatingParser field is automatically
// populated with a child parser that shares this parser's Validator.
func (vp *ValidatingParser) Next() (Payload, error) {
	// Drain any pending sub-parser from the previous ComponentSectionPayload.
	// If the caller already fully traversed it, this is a no-op (immediate EOF).
	if err := vp.drainPending(); err != nil {
		return nil, err
	}

	payload, err := vp.parser.Next()
	if err != nil {
		return nil, err
	}
	if err := vp.validator.ValidatePayload(payload); err != nil {
		return nil, err
	}
	// Validation consumes the BinaryReader in section payloads (via Items()).
	// Reset it so the caller can iterate the section again.
	if rp, ok := payload.(resettablePayload); ok {
		rp.resetReader()
	}
	// Set up the sub-ValidatingParser for nested components and track it
	// as pending so we can drain it if the caller doesn't.
	if cs, ok := payload.(*ComponentSectionPayload); ok {
		sub := &ValidatingParser{
			parser:    cs.Parser,
			validator: vp.validator,
		}
		cs.ValidatingParser = sub
		vp.pending = sub
	}

	// Wrap in validated variants where the validator has attached
	// handle information. Consume-and-clear the side-channel fields so
	// subsequent payloads don't accidentally pick up stale data.
	switch p := payload.(type) {
	case *ModuleSectionPayload:
		if vp.validator.lastValidatedModuleValid {
			wrapped := &ValidatedModuleSectionPayload{
				raw:        p,
				moduleType: vp.validator.lastValidatedModuleType,
				arena:      vp.validator.arena,
			}
			vp.validator.lastValidatedModuleValid = false
			return wrapped, nil
		}
	case *ComponentExportSectionPayload:
		entries := vp.validator.lastExportEntries
		vp.validator.lastExportEntries = nil
		return &ValidatedComponentExportSectionPayload{
			raw:     p,
			entries: entries,
			arena:   vp.validator.arena,
		}, nil
	case *ComponentImportSectionPayload:
		entries := vp.validator.lastImportEntries
		vp.validator.lastImportEntries = nil
		return &ValidatedComponentImportSectionPayload{
			raw:     p,
			entries: entries,
			arena:   vp.validator.arena,
		}, nil
	}
	return payload, nil
}

// drainPending consumes any remaining payloads from a pending sub-parser,
// validating each one. Returns the first error encountered.
func (vp *ValidatingParser) drainPending() error {
	if vp.pending == nil {
		return nil
	}
	pending := vp.pending
	vp.pending = nil
	for {
		_, err := pending.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Types returns completed type information after the entire component has
// been parsed and validated. Must be called after the outermost parser
// returns io.EOF.
func (vp *ValidatingParser) Types() (Types, error) {
	return vp.validator.Types()
}

// TopLevelComponentType returns an opaque handle to the outermost
// component's type after parsing has finished. Returns nil if parsing
// has not completed.
func (vp *ValidatingParser) TopLevelComponentType() *ComponentType {
	if !vp.validator.topLevelValid {
		return nil
	}
	return &ComponentType{
		arena: vp.validator.arena,
		id:    vp.validator.topLevelComponentTypeID,
	}
}
