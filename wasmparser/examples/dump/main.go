// Usage: dump <file.wasm>
//
// Reads a WebAssembly component binary and prints a detailed dump of every
// section and item, including types, imports, exports, aliases, canonical
// definitions, instances, and nested components/modules.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/partite-ai/wacogo/wasmparser"
)

var depth int

func indent() string {
	return strings.Repeat("  ", depth)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: dump <file.wasm>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var encodingStack []wasmparser.Encoding

	for payload, err := range wasmparser.ParseAll(f) {
		if err != nil {
			log.Fatalf("error: %v", err)
		}

		switch p := payload.(type) {
		case *wasmparser.VersionPayload:
			enc := "module"
			if p.Encoding == wasmparser.EncodingComponent {
				enc = "component"
			}
			fmt.Printf("%s(%s v%d\n", indent(), enc, p.Num)
			encodingStack = append(encodingStack, p.Encoding)
			depth++

		case *wasmparser.EndPayload:
			depth--
			if len(encodingStack) > 0 {
				encodingStack = encodingStack[:len(encodingStack)-1]
			}
			fmt.Printf("%s)\n", indent())

		case *wasmparser.ModuleSectionPayload:
			fmt.Printf("%s;; core module [%d-%d] (%d bytes)\n", indent(), p.Range.Start, p.Range.End, len(p.Data))

		case *wasmparser.ComponentSectionPayload:
			fmt.Printf("%s;; nested component [%d-%d]\n", indent(), p.Range.Start, p.Range.End)

		case *wasmparser.ComponentTypeSectionPayload:
			for ct, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printComponentType(ct)
			}

		case *wasmparser.ComponentImportSectionPayload:
			for imp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printImport(imp)
			}

		case *wasmparser.ComponentExportSectionPayload:
			for exp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printExport(exp)
			}

		case *wasmparser.ComponentAliasSectionPayload:
			for alias, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printAlias(alias)
			}

		case *wasmparser.ComponentCanonicalSectionPayload:
			for canon, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printCanonical(canon)
			}

		case *wasmparser.ComponentInstanceSectionPayload:
			for inst, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printComponentInstance(inst)
			}

		case *wasmparser.CoreTypeSectionPayload:
			for ct, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printCoreType(ct)
			}

		case *wasmparser.CoreInstanceSectionPayload:
			for inst, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printCoreInstance(inst)
			}

		case *wasmparser.ComponentStartSectionPayload:
			fmt.Printf("%s(start (func %d)", indent(), p.Start.FuncIndex)
			if len(p.Start.Args) > 0 {
				fmt.Printf(" (args")
				for _, a := range p.Start.Args {
					fmt.Printf(" %d", a)
				}
				fmt.Printf(")")
			}
			fmt.Printf(" (results %d))\n", p.Start.Results)

		case *wasmparser.ModuleTypeSectionPayload:
			for ft, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printModuleFuncType(ft)
			}

		case *wasmparser.ModuleImportSectionPayload:
			for imp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printModuleImport(imp)
			}

		case *wasmparser.ModuleFunctionSectionPayload:
			var indices []uint32
			for idx, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				indices = append(indices, idx)
			}
			fmt.Printf("%s(funcs %d type indices)\n", indent(), len(indices))

		case *wasmparser.ModuleTableSectionPayload:
			for tt, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printModuleTableType(tt)
			}

		case *wasmparser.ModuleMemorySectionPayload:
			for mt, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printModuleMemoryType(mt)
			}

		case *wasmparser.ModuleGlobalSectionPayload:
			fmt.Printf("%s(global %d bytes)\n", indent(), len(p.Data))

		case *wasmparser.ModuleExportSectionPayload:
			for exp, err := range p.Items() {
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				printModuleExport(exp)
			}

		case *wasmparser.ModuleStartSectionPayload:
			fmt.Printf("%s(start func %d)\n", indent(), p.FuncIndex)

		case *wasmparser.ModuleElementSectionPayload:
			fmt.Printf("%s(elem %d bytes)\n", indent(), len(p.Data))

		case *wasmparser.ModuleCodeSectionPayload:
			fmt.Printf("%s(code %d bytes)\n", indent(), len(p.Data))

		case *wasmparser.ModuleDataSectionPayload:
			fmt.Printf("%s(data %d bytes)\n", indent(), len(p.Data))

		case *wasmparser.ModuleDataCountSectionPayload:
			fmt.Printf("%s(data count %d)\n", indent(), p.Count)

		case *wasmparser.ModuleTagSectionPayload:
			fmt.Printf("%s(tag section)\n", indent())

		case *wasmparser.CustomSectionPayload:
			fmt.Printf("%s(custom %q %d bytes)\n", indent(), p.Name, len(p.Data))
		}
	}
}

func printComponentType(ct wasmparser.ComponentTypeDef) {
	switch t := ct.(type) {
	case *wasmparser.ComponentFuncType:
		fmt.Printf("%s(type (func", indent())
		if len(t.Params) > 0 {
			fmt.Printf(" (param")
			for _, p := range t.Params {
				fmt.Printf(" (;%s;) %s", p.Name, valTypeStr(p.Type))
			}
			fmt.Printf(")")
		}
		if len(t.Results) > 0 {
			fmt.Printf(" (result")
			for _, r := range t.Results {
				if r.Name != "" {
					fmt.Printf(" (;%s;) %s", r.Name, valTypeStr(r.Type))
				} else {
					fmt.Printf(" %s", valTypeStr(r.Type))
				}
			}
			fmt.Printf(")")
		}
		fmt.Println("))")

	case *wasmparser.ResourceType:
		rep := "i32"
		if t.Rep == wasmparser.ValTypeI64 {
			rep = "i64"
		}
		if t.Dtor.Valid {
			fmt.Printf("%s(type (resource (rep %s) (dtor (func %d))))\n", indent(), rep, t.Dtor.Value)
		} else {
			fmt.Printf("%s(type (resource (rep %s)))\n", indent(), rep)
		}

	case *wasmparser.ComponentTypeDecl:
		fmt.Printf("%s(type (component\n", indent())
		depth++
		for _, decl := range t.Declarations {
			printComponentTypeDecl(decl)
		}
		depth--
		fmt.Printf("%s))\n", indent())

	case *wasmparser.InstanceTypeDecl:
		fmt.Printf("%s(type (instance\n", indent())
		depth++
		for _, decl := range t.Declarations {
			printInstanceTypeDecl(decl)
		}
		depth--
		fmt.Printf("%s))\n", indent())

	case *wasmparser.ComponentDefinedTypeEntry:
		fmt.Printf("%s(type %s)\n", indent(), definedTypeStr(t.Type))

	default:
		fmt.Printf("%s(type ?unknown?)\n", indent())
	}
}

func printComponentTypeDecl(decl wasmparser.ComponentTypeDeclaration) {
	switch d := decl.(type) {
	case wasmparser.ComponentDeclImport:
		fmt.Printf("%s(import %q %s)\n", indent(), d.Import.Name.Name, typeRefStr(d.Import.Type))
	case wasmparser.InstanceDeclCoreType:
		fmt.Printf("%s(core type ...)\n", indent())
	case wasmparser.InstanceDeclType:
		printComponentType(d.Type)
	case wasmparser.InstanceDeclAlias:
		printAlias(d.Alias)
	case wasmparser.InstanceDeclExport:
		printExportDecl(d.Export)
	}
}

func printInstanceTypeDecl(decl wasmparser.InstanceTypeDeclaration) {
	switch d := decl.(type) {
	case wasmparser.InstanceDeclCoreType:
		fmt.Printf("%s(core type ...)\n", indent())
	case wasmparser.InstanceDeclType:
		printComponentType(d.Type)
	case wasmparser.InstanceDeclAlias:
		printAlias(d.Alias)
	case wasmparser.InstanceDeclExport:
		printExportDecl(d.Export)
	}
}

func printExportDecl(exp wasmparser.ComponentExport) {
	if exp.AscribedType.Valid {
		fmt.Printf("%s(export %q %s)\n", indent(), exp.Name.Name, typeRefStr(exp.AscribedType.Value))
	} else {
		fmt.Printf("%s(export %q (%s %d))\n", indent(), exp.Name.Name, externalKindStr(exp.Kind), exp.Index)
	}
}

func printImport(imp *wasmparser.ComponentImport) {
	fmt.Printf("%s(import %q %s)\n", indent(), imp.Name.Name, typeRefStr(imp.Type))
}

func printExport(exp *wasmparser.ComponentExport) {
	fmt.Printf("%s(export %q (%s %d)", indent(), exp.Name.Name, externalKindStr(exp.Kind), exp.Index)
	if exp.AscribedType.Valid {
		fmt.Printf(" (type %s)", typeRefStr(exp.AscribedType.Value))
	}
	fmt.Println(")")
}

func printAlias(alias wasmparser.ComponentAlias) {
	switch a := alias.(type) {
	case *wasmparser.AliasInstanceExport:
		fmt.Printf("%s(alias export %d %q (%s))\n", indent(), a.Instance, a.Name, externalKindStr(a.Kind))
	case *wasmparser.AliasCoreInstanceExport:
		fmt.Printf("%s(alias core export %d %q (%s))\n", indent(), a.Instance, a.Name, coreSortStr(a.Kind))
	case *wasmparser.AliasOuter:
		fmt.Printf("%s(alias outer %d %d (%s))\n", indent(), a.Count, a.Index, outerAliasKindStr(a.Kind))
	}
}

func printCanonical(canon wasmparser.CanonicalFunction) {
	switch c := canon.(type) {
	case *wasmparser.CanonLift:
		fmt.Printf("%s(canon lift (core func %d) (type %d)%s)\n",
			indent(), c.CoreFuncIndex, c.TypeIndex, canonOptsStr(c.Options))
	case *wasmparser.CanonLower:
		fmt.Printf("%s(canon lower (func %d)%s)\n",
			indent(), c.FuncIndex, canonOptsStr(c.Options))
	case *wasmparser.CanonResourceNew:
		fmt.Printf("%s(canon resource.new %d)\n", indent(), c.TypeIndex)
	case *wasmparser.CanonResourceDrop:
		fmt.Printf("%s(canon resource.drop %d)\n", indent(), c.TypeIndex)
	case *wasmparser.CanonResourceRep:
		fmt.Printf("%s(canon resource.rep %d)\n", indent(), c.TypeIndex)
	}
}

func printComponentInstance(inst wasmparser.ComponentInstance) {
	switch i := inst.(type) {
	case *wasmparser.Instantiate:
		fmt.Printf("%s(instance (instantiate %d", indent(), i.ComponentIndex)
		for _, arg := range i.Args {
			fmt.Printf("\n%s  (with %q (%s %d))", indent(), arg.Name, externalKindStr(arg.Kind), arg.Index)
		}
		fmt.Println("))")
	case *wasmparser.InstantiateFromExports:
		fmt.Printf("%s(instance\n", indent())
		for _, exp := range i.Exports {
			fmt.Printf("%s  (export %q (%s %d))\n", indent(), exp.Name.Name, externalKindStr(exp.Kind), exp.Index)
		}
		fmt.Printf("%s)\n", indent())
	}
}

func printCoreType(ct wasmparser.CoreType) {
	switch t := ct.(type) {
	case *wasmparser.CoreFuncType:
		fmt.Printf("%s(core type (func", indent())
		if len(t.Params) > 0 {
			fmt.Printf(" (param")
			for _, p := range t.Params {
				fmt.Printf(" %s", coreValStr(p))
			}
			fmt.Printf(")")
		}
		if len(t.Results) > 0 {
			fmt.Printf(" (result")
			for _, r := range t.Results {
				fmt.Printf(" %s", coreValStr(r))
			}
			fmt.Printf(")")
		}
		fmt.Println("))")
	default:
		fmt.Printf("%s(core type ...)\n", indent())
	}
}

func printCoreInstance(inst wasmparser.Instance) {
	switch i := inst.(type) {
	case *wasmparser.CoreInstantiate:
		fmt.Printf("%s(core instance (instantiate %d", indent(), i.ModuleIndex)
		for _, arg := range i.Args {
			fmt.Printf("\n%s  (with %q (%s %d))", indent(), arg.Name, coreSortStr(arg.Kind), arg.Index)
		}
		fmt.Println("))")
	case *wasmparser.CoreInstantiateFromExports:
		fmt.Printf("%s(core instance\n", indent())
		for _, exp := range i.Exports {
			fmt.Printf("%s  (export %q (%s %d))\n", indent(), exp.Name, coreSortStr(exp.Kind), exp.Index)
		}
		fmt.Printf("%s)\n", indent())
	}
}

// --- String formatting helpers ---

func valTypeStr(vt wasmparser.ComponentValType) string {
	switch v := vt.(type) {
	case wasmparser.PrimitiveValType:
		return primStr(v)
	case wasmparser.TypeIndexValType:
		return fmt.Sprintf("(type %d)", uint32(v))
	default:
		return "?"
	}
}

func primStr(p wasmparser.PrimitiveValType) string {
	switch p {
	case wasmparser.PrimBool:
		return "bool"
	case wasmparser.PrimS8:
		return "s8"
	case wasmparser.PrimU8:
		return "u8"
	case wasmparser.PrimS16:
		return "s16"
	case wasmparser.PrimU16:
		return "u16"
	case wasmparser.PrimS32:
		return "s32"
	case wasmparser.PrimU32:
		return "u32"
	case wasmparser.PrimS64:
		return "s64"
	case wasmparser.PrimU64:
		return "u64"
	case wasmparser.PrimF32:
		return "f32"
	case wasmparser.PrimF64:
		return "f64"
	case wasmparser.PrimChar:
		return "char"
	case wasmparser.PrimString:
		return "string"
	case wasmparser.PrimErrorContext:
		return "error-context"
	default:
		return fmt.Sprintf("prim(%d)", p)
	}
}

func definedTypeStr(dt wasmparser.ComponentDefinedType) string {
	switch t := dt.(type) {
	case *wasmparser.RecordType:
		fields := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = fmt.Sprintf("(field %q %s)", f.Name, valTypeStr(f.Type))
		}
		return fmt.Sprintf("(record %s)", strings.Join(fields, " "))
	case *wasmparser.VariantType:
		cases := make([]string, len(t.Cases))
		for i, c := range t.Cases {
			if c.Type.Valid {
				cases[i] = fmt.Sprintf("(case %q %s)", c.Name, valTypeStr(c.Type.Value))
			} else {
				cases[i] = fmt.Sprintf("(case %q)", c.Name)
			}
		}
		return fmt.Sprintf("(variant %s)", strings.Join(cases, " "))
	case *wasmparser.ListType:
		return fmt.Sprintf("(list %s)", valTypeStr(t.Element))
	case *wasmparser.TupleType:
		elems := make([]string, len(t.Types))
		for i, e := range t.Types {
			elems[i] = valTypeStr(e)
		}
		return fmt.Sprintf("(tuple %s)", strings.Join(elems, " "))
	case *wasmparser.FlagsType:
		return fmt.Sprintf("(flags %s)", strings.Join(quoted(t.Labels), " "))
	case *wasmparser.EnumType:
		return fmt.Sprintf("(enum %s)", strings.Join(quoted(t.Labels), " "))
	case *wasmparser.OptionType:
		return fmt.Sprintf("(option %s)", valTypeStr(t.Inner))
	case *wasmparser.ResultType:
		ok, errT := "", ""
		if t.Ok.Valid {
			ok = fmt.Sprintf(" (ok %s)", valTypeStr(t.Ok.Value))
		}
		if t.Err.Valid {
			errT = fmt.Sprintf(" (error %s)", valTypeStr(t.Err.Value))
		}
		return fmt.Sprintf("(result%s%s)", ok, errT)
	case *wasmparser.OwnType:
		return fmt.Sprintf("(own %d)", t.ResourceIndex)
	case *wasmparser.BorrowType:
		return fmt.Sprintf("(borrow %d)", t.ResourceIndex)
	case wasmparser.PrimitiveValType:
		return primStr(t)
	case wasmparser.TypeIndexValType:
		return fmt.Sprintf("(type %d)", uint32(t))
	default:
		return "?"
	}
}

func typeRefStr(tr wasmparser.ComponentTypeRef) string {
	switch t := tr.(type) {
	case wasmparser.TypeRefModule:
		return fmt.Sprintf("(module (type %d))", t.Index)
	case wasmparser.TypeRefFunc:
		return fmt.Sprintf("(func (type %d))", t.Index)
	case wasmparser.TypeRefValue:
		return fmt.Sprintf("(value %s)", valTypeStr(t.Type))
	case wasmparser.TypeRefType:
		switch b := t.Bounds.(type) {
		case wasmparser.TypeBoundsEq:
			return fmt.Sprintf("(type (eq %d))", b.Index)
		case wasmparser.TypeBoundsSubResource:
			return "(type (sub resource))"
		}
		return "(type ?)"
	case wasmparser.TypeRefComponent:
		return fmt.Sprintf("(component (type %d))", t.Index)
	case wasmparser.TypeRefInstance:
		return fmt.Sprintf("(instance (type %d))", t.Index)
	default:
		return "?"
	}
}

func externalKindStr(k wasmparser.ComponentExternalKind) string {
	switch k {
	case wasmparser.ExternalKindModule:
		return "module"
	case wasmparser.ExternalKindFunc:
		return "func"
	case wasmparser.ExternalKindValue:
		return "value"
	case wasmparser.ExternalKindType:
		return "type"
	case wasmparser.ExternalKindComponent:
		return "component"
	case wasmparser.ExternalKindInstance:
		return "instance"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

func coreSortStr(s wasmparser.CoreSort) string {
	switch s {
	case wasmparser.CoreSortFunc:
		return "func"
	case wasmparser.CoreSortTable:
		return "table"
	case wasmparser.CoreSortMemory:
		return "memory"
	case wasmparser.CoreSortGlobal:
		return "global"
	case wasmparser.CoreSortType:
		return "type"
	case wasmparser.CoreSortModule:
		return "module"
	case wasmparser.CoreSortInstance:
		return "instance"
	default:
		return fmt.Sprintf("sort(%d)", s)
	}
}

func outerAliasKindStr(k wasmparser.ComponentOuterAliasKind) string {
	switch k {
	case wasmparser.OuterAliasKindCoreModule:
		return "core module"
	case wasmparser.OuterAliasKindCoreType:
		return "core type"
	case wasmparser.OuterAliasKindType:
		return "type"
	case wasmparser.OuterAliasKindComponent:
		return "component"
	default:
		return fmt.Sprintf("outer(%d)", k)
	}
}

func canonOptsStr(opts []wasmparser.CanonicalOption) string {
	if len(opts) == 0 {
		return ""
	}
	var parts []string
	for _, o := range opts {
		switch opt := o.(type) {
		case wasmparser.CanonOptUTF8:
			parts = append(parts, "string-encoding=utf8")
		case wasmparser.CanonOptUTF16:
			parts = append(parts, "string-encoding=utf16")
		case wasmparser.CanonOptLatin1UTF16:
			parts = append(parts, "string-encoding=latin1+utf16")
		case wasmparser.CanonOptMemory:
			parts = append(parts, fmt.Sprintf("(memory %d)", opt.Index))
		case wasmparser.CanonOptRealloc:
			parts = append(parts, fmt.Sprintf("(realloc %d)", opt.Index))
		case wasmparser.CanonOptPostReturn:
			parts = append(parts, fmt.Sprintf("(post-return %d)", opt.Index))
		}
	}
	return " " + strings.Join(parts, " ")
}

func coreValStr(p wasmparser.CoreValParam) string {
	switch p.Byte {
	case 0x7F:
		return "i32"
	case 0x7E:
		return "i64"
	case 0x7D:
		return "f32"
	case 0x7C:
		return "f64"
	default:
		return fmt.Sprintf("0x%02x", p.Byte)
	}
}

func printModuleFuncType(ft *wasmparser.CoreFuncType) {
	fmt.Printf("%s(type (func", indent())
	if len(ft.Params) > 0 {
		fmt.Printf(" (param")
		for _, p := range ft.Params {
			fmt.Printf(" %s", coreValStr(p))
		}
		fmt.Printf(")")
	}
	if len(ft.Results) > 0 {
		fmt.Printf(" (result")
		for _, r := range ft.Results {
			fmt.Printf(" %s", coreValStr(r))
		}
		fmt.Printf(")")
	}
	fmt.Println("))")
}

func printModuleImport(imp wasmparser.ModuleImport) {
	desc := ""
	switch d := imp.Desc.(type) {
	case wasmparser.ImportDescFunc:
		desc = fmt.Sprintf("(func (type %d))", d.TypeIndex)
	case wasmparser.ImportDescTable:
		desc = fmt.Sprintf("(table %s %s)", coreValTypeStr(d.Type.ElemType), limitsStr32(d.Type.Min, d.Type.Max))
	case wasmparser.ImportDescMemory:
		desc = fmt.Sprintf("(memory %s)", limitsStr64(d.Type.Min, d.Type.Max))
	case wasmparser.ImportDescGlobal:
		desc = fmt.Sprintf("(global %s %s)", coreValTypeStr(d.Type.ValType), mutStr(d.Type.Mutable))
	case wasmparser.ImportDescTag:
		desc = fmt.Sprintf("(tag (type %d))", d.TypeIndex)
	}
	fmt.Printf("%s(import %q %q %s)\n", indent(), imp.Module, imp.Name, desc)
}

func printModuleExport(exp wasmparser.ModuleExport) {
	kindStr := ""
	switch exp.Kind {
	case 0x00:
		kindStr = "func"
	case 0x01:
		kindStr = "table"
	case 0x02:
		kindStr = "memory"
	case 0x03:
		kindStr = "global"
	case 0x04:
		kindStr = "tag"
	default:
		kindStr = fmt.Sprintf("kind(0x%02x)", exp.Kind)
	}
	fmt.Printf("%s(export %q (%s %d))\n", indent(), exp.Name, kindStr, exp.Index)
}

func printModuleTableType(tt wasmparser.CoreTableType) {
	fmt.Printf("%s(table %s %s)\n", indent(), coreValTypeStr(tt.ElemType), limitsStr32(tt.Min, tt.Max))
}

func printModuleMemoryType(mt wasmparser.CoreMemoryType) {
	fmt.Printf("%s(memory %s)\n", indent(), limitsStr64(mt.Min, mt.Max))
}

func coreValTypeStr(vt wasmparser.CoreValType) string {
	switch vt {
	case wasmparser.CoreValTypeI32:
		return "i32"
	case wasmparser.CoreValTypeI64:
		return "i64"
	case wasmparser.CoreValTypeF32:
		return "f32"
	case wasmparser.CoreValTypeF64:
		return "f64"
	case wasmparser.CoreValTypeV128:
		return "v128"
	case wasmparser.CoreValTypeFuncRef:
		return "funcref"
	case wasmparser.CoreValTypeExternRef:
		return "externref"
	default:
		return fmt.Sprintf("type(%d)", vt)
	}
}

func mutStr(mutable bool) string {
	if mutable {
		return "(mut)"
	}
	return ""
}

func limitsStr32(min uint32, max wasmparser.Optional[uint32]) string {
	if max.Valid {
		return fmt.Sprintf("%d %d", min, max.Value)
	}
	return fmt.Sprintf("%d", min)
}

func limitsStr64(min uint64, max wasmparser.Optional[uint64]) string {
	if max.Valid {
		return fmt.Sprintf("%d %d", min, max.Value)
	}
	return fmt.Sprintf("%d", min)
}

func quoted(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}
