package witgen

import (
	"bytes"
	"fmt"
)

// ifaceView is the template input for iface.tmpl. Wraps the IR
// Interface with derived fields the template needs.
type ifaceView struct {
	*Interface
	IfaceGoName string
	ImplName    string
	WrapName    string
	Tuples      []*TypeTuple
	Options     []*TypeOption
	Results     []*TypeResult
	// ParentFuncs is the unified list of signatures that should appear
	// on the parent (interface-level) Go interface: freestanding funcs
	// plus hoisted resource constructors and statics.
	ParentFuncs []parentFunc
	SourceFile  string
}

// parentFunc is one entry on the parent (interface-level) Go interface.
// After the handle-management refactor, the host-impl side and the
// caller-wrapper side share a single signature using *<R>Handle for
// resource types, so only one Sig is needed.
type parentFunc struct {
	Sig  string
	Docs string
}

// emitIfaceFile produces the full text of <iface>.go for one Interface.
// Returns formatted Go source bytes ready to write to disk.
func emitIfaceFile(iface *Interface, sourceFile string) ([]byte, error) {
	var buf bytes.Buffer

	headerView := ifaceView{
		Interface:  iface,
		SourceFile: sourceFile,
	}
	header, err := renderTemplate("file_header.tmpl", headerView)
	if err != nil {
		return nil, err
	}
	buf.Write(header)
	buf.WriteString("\n")

	// Emit a superset import block. formatGoSource prunes the imports
	// the file doesn't actually reference.
	buf.WriteString("import (\n")
	buf.WriteString("\t\"context\"\n")
	buf.WriteString("\t\"fmt\"\n\n")
	buf.WriteString("\t\"github.com/partite-ai/wacogo\"\n")
	buf.WriteString("\t\"github.com/partite-ai/wacogo/host\"\n")
	for _, imp := range iface.Imports {
		if imp.GoPackagePath != "" {
			fmt.Fprintf(&buf, "\t%s %q\n", imp.GoPackage, imp.GoPackagePath)
		}
	}
	buf.WriteString(")\n\n")

	types := CollectTypes(iface)
	view := ifaceView{
		Interface:   iface,
		IfaceGoName: iface.GoName,
		ImplName:    iface.ImplName,
		WrapName:    iface.WrapName,
		Tuples:      TupleTypes(types),
		Options:     OptionTypes(types),
		Results:     ResultTypes(types),
		SourceFile:  sourceFile,
	}
	for _, f := range iface.Funcs {
		view.ParentFuncs = append(view.ParentFuncs, parentFunc{
			Sig:  funcSigForSide(false, f),
			Docs: f.Docs,
		})
	}
	for _, rt := range iface.Resources {
		if rt.Ctor != nil {
			view.ParentFuncs = append(view.ParentFuncs, parentFunc{
				Sig:  ctorSigForSide(false, rt),
				Docs: rt.Ctor.Docs,
			})
		}
		for _, s := range rt.Statics {
			view.ParentFuncs = append(view.ParentFuncs, parentFunc{
				Sig:  staticSigForSide(false, rt, s),
				Docs: s.Docs,
			})
		}
	}
	body, err := renderTemplate("iface.tmpl", view)
	if err != nil {
		return nil, err
	}
	buf.Write(body)

	return formatGoSource(iface.GoPackage+".go", buf.Bytes())
}
