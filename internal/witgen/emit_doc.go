package witgen

import "strings"

// goDocComment formats raw WIT doc text into a Go doc-comment block.
//
// Returns the empty string if text is empty. Otherwise:
//   - splits text on '\n';
//   - strips leading and trailing blank lines (lines that are empty after
//     trimming trailing whitespace);
//   - collapses runs of 2+ blank lines into a single blank line;
//   - prefixes each non-blank line with indent + "// " and each blank line
//     with indent + "//";
//   - terminates the output with a single trailing '\n'.
//
// indent is the leading whitespace to apply to every emitted line. Use ""
// for top-level declarations and "\t" for struct fields.
func goDocComment(text, indent string) string {
	if text == "" {
		return ""
	}
	rawLines := strings.Split(text, "\n")
	// Detect blank lines (treat trailing whitespace as blank).
	isBlank := make([]bool, len(rawLines))
	for i, ln := range rawLines {
		isBlank[i] = strings.TrimRight(ln, " \t") == ""
	}
	// Strip leading blanks.
	start := 0
	for start < len(rawLines) && isBlank[start] {
		start++
	}
	// Strip trailing blanks.
	end := len(rawLines)
	for end > start && isBlank[end-1] {
		end--
	}
	if start == end {
		return ""
	}
	var b strings.Builder
	prevBlank := false
	for i := start; i < end; i++ {
		blank := isBlank[i]
		if blank {
			if prevBlank {
				continue // collapse runs
			}
			b.WriteString(indent)
			b.WriteString("//\n")
			prevBlank = true
			continue
		}
		b.WriteString(indent)
		b.WriteString("// ")
		b.WriteString(rawLines[i])
		b.WriteByte('\n')
		prevBlank = false
	}
	return b.String()
}
