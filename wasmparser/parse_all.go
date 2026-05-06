package wasmparser

import (
	"io"
	"iter"
)

// ParseAll reads a WebAssembly binary from r and yields all Payload values,
// including payloads from nested components and modules.
func ParseAll(r io.Reader) iter.Seq2[Payload, error] {
	return func(yield func(Payload, error) bool) {
		stack := []*Parser{NewParser(r)}
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			payload, err := top.Next()
			if err == io.EOF {
				stack = stack[:len(stack)-1]
				continue
			}
			if err != nil {
				yield(nil, err)
				return
			}

			switch p := payload.(type) {
			case *ComponentSectionPayload:
				if !yield(payload, nil) {
					return
				}
				stack = append(stack, p.Parser)
			case *ModuleSectionPayload:
				if !yield(payload, nil) {
					return
				}
				stack = append(stack, p.Parser)
			default:
				if !yield(payload, nil) {
					return
				}
			}
		}
	}
}

