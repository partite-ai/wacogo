package witgen

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"go.bytecodealliance.org/wit"
)

// LowerWorld walks the chosen world's imported interfaces and lowers
// each to the witgen IR. worldID is the fully-qualified world name
// "ns:pkg/world". packageRoot is the Go module path prefix used to
// compute GoPackagePath in ImportRef (may be empty, in which case
// GoPackagePath is left unset).
func LowerWorld(res *wit.Resolve, worldID string, packageRoot ...string) (*Package, error) {
	w, err := findWorld(res, worldID)
	if err != nil {
		return nil, err
	}

	root := ""
	if len(packageRoot) > 0 {
		root = packageRoot[0]
	}

	pkg := &Package{}
	imports := collectWorldImports(w)
	names := make([]string, 0, len(imports))
	for k := range imports {
		names = append(names, k)
	}
	sort.Strings(names)

	// Collect all *wit.Interface values in deterministic order so we
	// can build a global typeDefIR map before lowering any functions.
	type entry struct {
		name string
		wif  *wit.Interface
	}
	var entries []entry
	for _, name := range names {
		item := imports[name]
		wif, err := importInterface(item)
		if err != nil {
			return nil, fmt.Errorf("witgen: world %q import %q: %w", worldID, name, err)
		}
		entries = append(entries, entry{name, wif})
	}

	// Pre-pass: create shallow IR stubs for all named nominal TypeDefs
	// (resource, variant, record, enum, flags) across all interfaces.
	// This lets cross-interface type references resolve to stable IR
	// pointers during the main lowering pass.
	globalTypeDefIR := make(map[*wit.TypeDef]Type)
	for _, e := range entries {
		if err := preRegisterNominals(e.wif, globalTypeDefIR); err != nil {
			return nil, fmt.Errorf("witgen: pre-register %q: %w", e.name, err)
		}
	}

	// Main lowering pass.
	for _, e := range entries {
		iface, err := lowerInterface(e.wif, globalTypeDefIR)
		if err != nil {
			return nil, fmt.Errorf("witgen: import %q: %w", e.name, err)
		}
		pkg.Interfaces = append(pkg.Interfaces, iface)
	}

	// Final pass: populate cross-interface imports now that all
	// interfaces (and their Resources slices) are fully populated.
	for _, iface := range pkg.Interfaces {
		collectImports(iface, pkg.Interfaces, root)
	}

	// Post-pass: for each interface that has imports, replace shared
	// IR pointers in function/method/ctor/static signatures with
	// per-interface qualifier-tagged copies. Resources additionally
	// gain ImportRefField + LenderInstField; nominals (record, variant,
	// enum, flags) gain GoPackageQualifier so signature renderers emit
	// `<pkg>.<Type>` references. Recurses through compounds.
	for _, iface := range pkg.Interfaces {
		if len(iface.Imports) > 0 {
			rewriteImports(iface)
		}
	}

	return pkg, nil
}

// preRegisterNominals walks wif's TypeDefs and creates shallow IR stubs for
// every named nominal type — resource, variant, record, enum, flags — in
// globalTypeDefIR. This pre-pass must run for all interfaces before any
// lowerInterface call so that cross-interface type references (e.g. an
// option<ip-address> in ip-name-lookup referencing a variant declared in
// network) can be resolved during the main lowering pass.
func preRegisterNominals(wif *wit.Interface, globalTypeDefIR map[*wit.TypeDef]Type) error {
	wif.TypeDefs.All()(func(_ string, td *wit.TypeDef) bool {
		if td.Name == nil {
			return true
		}
		if _, exists := globalTypeDefIR[td]; exists {
			return true // already registered
		}
		tdName := *td.Name
		goName := GoName(tdName, true)
		switch td.Kind.(type) {
		case *wit.Resource:
			globalTypeDefIR[td] = &TypeResource{
				Name:     tdName,
				GoName:   goName,
				WrapName: goName,
				ImplName: goName + "Impl",
			}
		case *wit.Variant:
			globalTypeDefIR[td] = &TypeVariant{
				Name:   tdName,
				GoName: goName,
			}
		case *wit.Record:
			globalTypeDefIR[td] = &TypeRecord{
				Name:   tdName,
				GoName: goName,
			}
		case *wit.Enum:
			globalTypeDefIR[td] = &TypeEnum{
				Name:   tdName,
				GoName: goName,
			}
		case *wit.Flags:
			globalTypeDefIR[td] = &TypeFlags{
				Name:   tdName,
				GoName: goName,
			}
		}
		return true
	})
	return nil
}

// findOwnerInterface returns the Interface that owns r (i.e., r appears
// in iface.Resources), or nil if r is not owned by any of the given
// interfaces.
func findOwnerInterface(r *TypeResource, all []*Interface) *Interface {
	for _, iface := range all {
		for _, x := range iface.Resources {
			if x == r {
				return iface
			}
		}
	}
	return nil
}

// collectImports walks iface's signatures and collects *TypeResource
// references whose owning interface is different from iface. One
// ImportRef is produced per distinct owner interface; resources within
// an ImportRef are deduped and the final slice is sorted for
// determinism.
func collectImports(iface *Interface, all []*Interface, packageRoot string) {
	seen := make(map[*Interface]*ImportRef)

	addResource := func(r *TypeResource) {
		owner := findOwnerInterface(r, all)
		if owner == nil || owner == iface {
			return
		}
		ref, exists := seen[owner]
		if !exists {
			goPackagePath := ""
			if packageRoot != "" {
				goPackagePath = path.Join(packageRoot, owner.Namespace, owner.Package, owner.GoPackage)
			}
			ref = &ImportRef{
				Namespace:     owner.Namespace,
				Package:       owner.Package,
				Name:          owner.Name,
				GoPackage:     owner.GoPackage,
				GoPackagePath: goPackagePath,
				GoFieldName:   GoName(owner.Name, true),
				StateField:    GoName(owner.Name, false),
			}
			seen[owner] = ref
		}
		// Dedup resources within this ref.
		for _, ir := range ref.Resources {
			if ir.Name == r.Name {
				return
			}
		}
		ref.Resources = append(ref.Resources, ImportedResource{
			Name:          r.Name,
			WrapName:      r.WrapName,
			RefField:      r.WrapName + "Ref",
			ForwarderType: ref.GoPackage + r.WrapName + "Forwarder",
			FwdFnsType:    ref.GoPackage + r.WrapName + "FwdFns",
			FwdFnsField:   ref.GoPackage + r.WrapName + "FwdFns",
			Source:        r,
		})
	}

	transitive := false
	addNominal := func(src Type, name, goName string, owner *Interface, slot func(*ImportRef) *[]ImportedNominal) {
		if owner == nil || owner == iface {
			return
		}
		ref, exists := seen[owner]
		if !exists {
			goPackagePath := ""
			if packageRoot != "" {
				goPackagePath = path.Join(packageRoot, owner.Namespace, owner.Package, owner.GoPackage)
			}
			ref = &ImportRef{
				Namespace:     owner.Namespace,
				Package:       owner.Package,
				Name:          owner.Name,
				GoPackage:     owner.GoPackage,
				GoPackagePath: goPackagePath,
				GoFieldName:   GoName(owner.Name, true),
				StateField:    GoName(owner.Name, false),
			}
			seen[owner] = ref
		}
		bucket := slot(ref)
		for i, n := range *bucket {
			if n.Name == name {
				// Promote transitive→direct on subsequent direct sighting.
				if !transitive && n.Transitive {
					(*bucket)[i].Transitive = false
				}
				return
			}
		}
		*bucket = append(*bucket, ImportedNominal{Name: name, GoName: goName, Source: src, Transitive: transitive})
	}

	addRecord := func(r *TypeRecord) {
		addNominal(r, r.Name, r.GoName, r.OwnerInterface, func(ref *ImportRef) *[]ImportedNominal { return &ref.Records })
	}
	addVariant := func(v *TypeVariant) {
		addNominal(v, v.Name, v.GoName, v.OwnerInterface, func(ref *ImportRef) *[]ImportedNominal { return &ref.Variants })
	}
	addEnum := func(e *TypeEnum) {
		addNominal(e, e.Name, e.GoName, e.OwnerInterface, func(ref *ImportRef) *[]ImportedNominal { return &ref.Enums })
	}
	addFlags := func(f *TypeFlags) {
		addNominal(f, f.Name, f.GoName, f.OwnerInterface, func(ref *ImportRef) *[]ImportedNominal { return &ref.Flags })
	}

	var walk func(t Type)
	walk = func(t Type) {
		switch v := t.(type) {
		case *TypeResource:
			addResource(v)
		case *TypeOwn:
			addResource(v.Resource)
		case *TypeBorrow:
			addResource(v.Resource)
		case *TypeRecord:
			addRecord(v)
			for _, f := range v.Fields {
				walk(f.Type)
			}
		case *TypeVariant:
			addVariant(v)
			for _, c := range v.Cases {
				if c.Payload != nil {
					walk(c.Payload)
				}
			}
		case *TypeEnum:
			addEnum(v)
		case *TypeFlags:
			addFlags(v)
		case *TypeList:
			walk(v.Elem)
		case *TypeOption:
			walk(v.Elem)
		case *TypeResult:
			walk(v.OK)
			walk(v.Err)
		case *TypeTuple:
			for _, f := range v.Fields {
				walk(f)
			}
		}
	}

	visitFunc := func(f *Func) {
		for _, p := range f.Params {
			walk(p.Type)
		}
		if f.Result != nil {
			walk(f.Result)
		}
	}

	for _, f := range iface.Funcs {
		visitFunc(f)
	}

	for _, r := range iface.Resources {
		if r.Ctor != nil {
			for _, p := range r.Ctor.Params {
				walk(p.Type)
			}
		}
		for _, m := range r.Methods {
			for _, p := range m.Params {
				walk(p.Type)
			}
			if m.Result != nil {
				walk(m.Result)
			}
		}
		for _, s := range r.Statics {
			for _, p := range s.Params {
				walk(p.Type)
			}
			if s.Result != nil {
				walk(s.Result)
			}
		}
	}

	// Transitive walk: imported resources' methods may reference nominals
	// from other packages (e.g. filesystem imports streams.InputStream
	// whose Read method returns result<list<u8>, streams.StreamError>).
	// Mark nominals added during this walk as Transitive so factory
	// emission skips host.AddType for them — they're only needed for
	// qualifier-copy bookkeeping in rewriteImports.
	transitive = true
	visited := map[*TypeResource]bool{}
	for {
		var pending []*TypeResource
		for _, ref := range seen {
			for _, ir := range ref.Resources {
				if ir.Source == nil || visited[ir.Source] {
					continue
				}
				visited[ir.Source] = true
				pending = append(pending, ir.Source)
			}
		}
		if len(pending) == 0 {
			break
		}
		for _, r := range pending {
			if r.Ctor != nil {
				for _, p := range r.Ctor.Params {
					walk(p.Type)
				}
			}
			for _, m := range r.Methods {
				for _, p := range m.Params {
					walk(p.Type)
				}
				if m.Result != nil {
					walk(m.Result)
				}
			}
			for _, s := range r.Statics {
				for _, p := range s.Params {
					walk(p.Type)
				}
				if s.Result != nil {
					walk(s.Result)
				}
			}
		}
	}
	transitive = false

	// Flatten map → sorted slice for determinism.
	iface.Imports = make([]ImportRef, 0, len(seen))
	for _, ref := range seen {
		iface.Imports = append(iface.Imports, *ref)
	}
	sort.Slice(iface.Imports, func(i, j int) bool {
		return iface.Imports[i].Name < iface.Imports[j].Name
	})
}

// rewriteImports replaces imported type pointers in iface's function and
// resource signatures with per-interface shallow copies that carry a
// GoPackageQualifier. For resources the copy also gets ImportRefField and
// LenderInstField set. The originals (owned by the source interface) are
// never mutated. Compound types (option, list, tuple, result) are
// recursively rewritten so their element types are updated too.
func rewriteImports(iface *Interface) {
	resCopies := make(map[*TypeResource]*TypeResource)
	recCopies := make(map[*TypeRecord]*TypeRecord)
	varCopies := make(map[*TypeVariant]*TypeVariant)
	enumCopies := make(map[*TypeEnum]*TypeEnum)
	flagsCopies := make(map[*TypeFlags]*TypeFlags)

	for _, imp := range iface.Imports {
		for _, ir := range imp.Resources {
			// Prefer the back-pointer Source (set by collectImports) so we
			// also build copies for resources reached only via the
			// transitive walk through imported resources' methods.
			orig := ir.Source
			if orig == nil {
				orig = findImportedResource(iface, ir.Name, ir.WrapName)
			}
			if orig == nil || orig.ImportRefField != "" {
				continue
			}
			refField := strings.ToLower(ir.RefField[:1]) + ir.RefField[1:]
			cp := *orig
			cp.ImportRefField = refField
			cp.GoPackageQualifier = imp.GoPackage
			cp.LenderInstField = imp.StateField
			resCopies[orig] = &cp
		}
		for _, n := range imp.Records {
			orig := n.Source.(*TypeRecord)
			if _, exists := recCopies[orig]; exists {
				continue
			}
			cp := *orig
			cp.GoPackageQualifier = imp.GoPackage
			recCopies[orig] = &cp
		}
		for _, n := range imp.Variants {
			orig := n.Source.(*TypeVariant)
			if _, exists := varCopies[orig]; exists {
				continue
			}
			cp := *orig
			cp.GoPackageQualifier = imp.GoPackage
			varCopies[orig] = &cp
		}
		for _, n := range imp.Enums {
			orig := n.Source.(*TypeEnum)
			if _, exists := enumCopies[orig]; exists {
				continue
			}
			cp := *orig
			cp.GoPackageQualifier = imp.GoPackage
			enumCopies[orig] = &cp
		}
		for _, n := range imp.Flags {
			orig := n.Source.(*TypeFlags)
			if _, exists := flagsCopies[orig]; exists {
				continue
			}
			cp := *orig
			cp.GoPackageQualifier = imp.GoPackage
			flagsCopies[orig] = &cp
		}
	}

	var rewrite func(t Type) Type
	rewrite = func(t Type) Type {
		switch v := t.(type) {
		case *TypeOwn:
			if cp, ok := resCopies[v.Resource]; ok {
				return &TypeOwn{Resource: cp}
			}
		case *TypeBorrow:
			if cp, ok := resCopies[v.Resource]; ok {
				return &TypeBorrow{Resource: cp}
			}
		case *TypeRecord:
			if cp, ok := recCopies[v]; ok {
				return cp
			}
		case *TypeVariant:
			if cp, ok := varCopies[v]; ok {
				return cp
			}
		case *TypeEnum:
			if cp, ok := enumCopies[v]; ok {
				return cp
			}
		case *TypeFlags:
			if cp, ok := flagsCopies[v]; ok {
				return cp
			}
		case *TypeOption:
			return &TypeOption{Elem: rewrite(v.Elem)}
		case *TypeList:
			return &TypeList{Elem: rewrite(v.Elem)}
		case *TypeTuple:
			fields := make([]Type, len(v.Fields))
			for i, f := range v.Fields {
				fields[i] = rewrite(f)
			}
			return &TypeTuple{Fields: fields, GoPackageQualifier: v.GoPackageQualifier}
		case *TypeResult:
			out := &TypeResult{}
			if v.OK != nil {
				out.OK = rewrite(v.OK)
			}
			if v.Err != nil {
				out.Err = rewrite(v.Err)
			}
			return out
		}
		return t
	}

	// tagAnonymous recursively tags TypeTuple and TypeOption nodes in a type
	// with pkg, so that anonymous compound types embedded in imported nominal
	// types are rendered with the owning package qualifier rather than as
	// local structs/helpers.
	//
	// Every option<T> nested inside an imported record or variant case is tagged
	// with the owning package qualifier, regardless of whether T is a primitive
	// or a nominal type. The qualified option name uses localTypeName for the
	// elem so that e.g. option<FieldSizePayload> from the types package becomes
	// TypesOptionFieldSizePayload (→ types.OptionFieldSizePayload in Go), which
	// matches the struct that actually lives in the types package.
	var tagAnonymous func(t Type, pkg string) Type
	tagAnonymous = func(t Type, pkg string) Type {
		switch v := t.(type) {
		case *TypeTuple:
			fields := make([]Type, len(v.Fields))
			for i, f := range v.Fields {
				fields[i] = tagAnonymous(f, pkg)
			}
			return &TypeTuple{Fields: fields, GoPackageQualifier: pkg}
		case *TypeOption:
			elem := tagAnonymous(v.Elem, pkg)
			return &TypeOption{Elem: elem, GoPackageQualifier: pkg}
		case *TypeList:
			return &TypeList{Elem: tagAnonymous(v.Elem, pkg)}
		case *TypeResult:
			out := &TypeResult{GoPackageQualifier: pkg}
			if v.OK != nil {
				out.OK = tagAnonymous(v.OK, pkg)
			}
			if v.Err != nil {
				out.Err = tagAnonymous(v.Err, pkg)
			}
			return out
		}
		return t
	}

	// Rewrite the interior of each per-iface record/variant copy so the
	// copy's fields/case-payloads point at qualifier-tagged copies, not
	// the originals (which still belong to the owner package).
	for _, cp := range recCopies {
		newFields := make([]RecordField, len(cp.Fields))
		for i, f := range cp.Fields {
			ft := rewrite(f.Type)
			if cp.GoPackageQualifier != "" {
				ft = tagAnonymous(ft, cp.GoPackageQualifier)
			}
			newFields[i] = RecordField{Name: f.Name, GoName: f.GoName, Type: ft}
		}
		cp.Fields = newFields
	}
	for _, cp := range varCopies {
		newCases := make([]VariantCase, len(cp.Cases))
		for i, c := range cp.Cases {
			out := VariantCase{Name: c.Name, GoName: c.GoName}
			if c.Payload != nil {
				p := rewrite(c.Payload)
				// Tag any tuple payloads with the variant's package qualifier so
				// that helpers in the importing package reference e.g.
				// network.TupleU8U8U8U8 instead of a local TupleU8U8U8U8.
				if cp.GoPackageQualifier != "" {
					p = tagAnonymous(p, cp.GoPackageQualifier)
				}
				out.Payload = p
			}
			newCases[i] = out
		}
		cp.Cases = newCases
	}

	// Build per-importer qualified copies of imported resources' methods
	// so the importer-emitted forwarder bodies can reference foreign
	// nominal types via their owning package qualifier.
	for impIdx := range iface.Imports {
		imp := &iface.Imports[impIdx]
		for irIdx := range imp.Resources {
			ir := &imp.Resources[irIdx]
			if ir.Source == nil {
				continue
			}
			qm := make([]ResourceMethod, len(ir.Source.Methods))
			for i, m := range ir.Source.Methods {
				cp := ResourceMethod{
					Name:   m.Name,
					GoName: m.GoName,
					Docs:   m.Docs,
					Params: make([]*Param, len(m.Params)),
				}
				for j, p := range m.Params {
					t := rewrite(p.Type)
					t = tagAnonymous(t, imp.GoPackage)
					cp.Params[j] = &Param{GoName: p.GoName, Type: t}
				}
				if m.Result != nil {
					t := rewrite(m.Result)
					t = tagAnonymous(t, imp.GoPackage)
					cp.Result = t
				}
				qm[i] = cp
			}
			ir.QualifiedMethods = qm
		}
	}

	for _, f := range iface.Funcs {
		for _, p := range f.Params {
			p.Type = rewrite(p.Type)
		}
		if f.Result != nil {
			f.Result = rewrite(f.Result)
		}
	}
	for _, r := range iface.Resources {
		if r.Ctor != nil {
			for _, p := range r.Ctor.Params {
				p.Type = rewrite(p.Type)
			}
		}
		for i := range r.Methods {
			for _, p := range r.Methods[i].Params {
				p.Type = rewrite(p.Type)
			}
			if r.Methods[i].Result != nil {
				r.Methods[i].Result = rewrite(r.Methods[i].Result)
			}
		}
		for i := range r.Statics {
			for _, p := range r.Statics[i].Params {
				p.Type = rewrite(p.Type)
			}
			if r.Statics[i].Result != nil {
				r.Statics[i].Result = rewrite(r.Statics[i].Result)
			}
		}
	}
	// Also rewrite case payloads in locally-defined variants and fields in
	// locally-defined records, so compound types whose components reference
	// imported resources/nominals get proper package-qualified copies.
	for _, v := range iface.Variants {
		for i, c := range v.Cases {
			if c.Payload != nil {
				v.Cases[i].Payload = rewrite(c.Payload)
			}
		}
	}
	for _, r := range iface.Records {
		for i, f := range r.Fields {
			r.Fields[i].Type = rewrite(f.Type)
		}
	}
}

// findImportedResource searches iface's function/resource signatures for a
// *TypeResource with the given name and wrapName, and returns the first
// match found. Used to locate the shared pointer before copying it.
func findImportedResource(iface *Interface, name, wrapName string) *TypeResource {
	var findInType func(t Type) *TypeResource
	findInType = func(t Type) *TypeResource {
		switch v := t.(type) {
		case *TypeOwn:
			if v.Resource.Name == name && v.Resource.WrapName == wrapName {
				return v.Resource
			}
		case *TypeBorrow:
			if v.Resource.Name == name && v.Resource.WrapName == wrapName {
				return v.Resource
			}
		case *TypeOption:
			return findInType(v.Elem)
		case *TypeResult:
			if v.OK != nil {
				if r := findInType(v.OK); r != nil {
					return r
				}
			}
			if v.Err != nil {
				return findInType(v.Err)
			}
		case *TypeList:
			return findInType(v.Elem)
		case *TypeTuple:
			for _, f := range v.Fields {
				if r := findInType(f); r != nil {
					return r
				}
			}
		case *TypeVariant:
			for _, c := range v.Cases {
				if c.Payload != nil {
					if r := findInType(c.Payload); r != nil {
						return r
					}
				}
			}
		case *TypeRecord:
			for _, f := range v.Fields {
				if r := findInType(f.Type); r != nil {
					return r
				}
			}
		}
		return nil
	}
	for _, f := range iface.Funcs {
		for _, p := range f.Params {
			if r := findInType(p.Type); r != nil {
				return r
			}
		}
		if f.Result != nil {
			if r := findInType(f.Result); r != nil {
				return r
			}
		}
	}
	for _, res := range iface.Resources {
		if res.Ctor != nil {
			for _, p := range res.Ctor.Params {
				if r := findInType(p.Type); r != nil {
					return r
				}
			}
		}
		for _, m := range res.Methods {
			for _, p := range m.Params {
				if r := findInType(p.Type); r != nil {
					return r
				}
			}
			if m.Result != nil {
				if r := findInType(m.Result); r != nil {
					return r
				}
			}
		}
		for _, s := range res.Statics {
			for _, p := range s.Params {
				if r := findInType(p.Type); r != nil {
					return r
				}
			}
			if s.Result != nil {
				if r := findInType(s.Result); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

// collectWorldImports flattens the ordered.Map of world imports into a
// plain Go map keyed by the import name.
func collectWorldImports(w *wit.World) map[string]wit.WorldItem {
	out := make(map[string]wit.WorldItem, w.Imports.Len())
	w.Imports.All()(func(k string, v wit.WorldItem) bool {
		out[k] = v
		return true
	})
	return out
}

// importInterface unwraps a world-import value into a *wit.Interface, or
// errors if the import is something unsupported (type import, freestanding
// function, etc.).
func importInterface(item wit.WorldItem) (*wit.Interface, error) {
	switch v := item.(type) {
	case *wit.InterfaceRef:
		if v.Interface == nil {
			return nil, fmt.Errorf("interface ref has nil Interface")
		}
		return v.Interface, nil
	default:
		return nil, fmt.Errorf("import is %T, not an interface (only interface imports are supported)", item)
	}
}

func findWorld(res *wit.Resolve, worldID string) (*wit.World, error) {
	slash := strings.LastIndex(worldID, "/")
	if slash < 0 {
		return nil, fmt.Errorf("witgen: world id %q must be of form ns:pkg/name", worldID)
	}
	pkgPart := worldID[:slash]
	nameSought := worldID[slash+1:]
	for _, w := range res.Worlds {
		if w.Name != nameSought {
			continue
		}
		if w.Package == nil {
			continue
		}
		if pkgQualName(w.Package) != pkgPart {
			continue
		}
		return w, nil
	}
	return nil, fmt.Errorf("witgen: world %q not found in resolve", worldID)
}

// pkgQualName extracts the unversioned "ns:pkg" qualified name from a
// *wit.Package's Ident.
func pkgQualName(p *wit.Package) string {
	id := p.Name
	id.Extension = ""
	id.Version = nil
	return id.String()
}

// interfaceIdent returns the canonical WIT interface id
// ("ns:pkg/name" or "ns:pkg/name@version") for wif. Used to populate
// Interface.InterfaceName.
func interfaceIdent(wif *wit.Interface) string {
	if wif.Package == nil || wif.Name == nil {
		return ""
	}
	id := wif.Package.Name
	id.Extension = *wif.Name
	return id.String()
}

func lowerInterface(wif *wit.Interface, globalTypeDefIR map[*wit.TypeDef]Type) (*Interface, error) {
	if wif.Name == nil {
		return nil, fmt.Errorf("anonymous interface — not yet supported")
	}
	name := *wif.Name
	goName := GoName(name, true)
	iface := &Interface{
		Name:      name,
		GoName:    goName,
		WrapName:  goName,
		ImplName:  goName + "Impl",
		GoPackage: GoPackageName(name),
	}
	if wif.Package != nil {
		iface.Namespace = wif.Package.Name.Namespace
		iface.Package = wif.Package.Name.Package
		iface.InterfaceName = interfaceIdent(wif)
	}
	iface.Docs = wif.Docs.Contents

	// First pass: collect nominal type declarations (enum, flags, resources)
	// so subsequent function-signature lowering can resolve references to
	// the same IR pointer via TypeDef identity. Resources are pre-registered
	// in globalTypeDefIR; we copy the whole global map as our starting
	// typeDefIR so cross-interface own<R>/borrow<R> can resolve.
	//
	// Iterate TypeDefs in declaration order (ordered.Map.All) so the
	// emitter gets a stable output shape.
	typeDefIR := make(map[*wit.TypeDef]Type, len(globalTypeDefIR))
	for k, v := range globalTypeDefIR {
		typeDefIR[k] = v
	}
	var typeDefErr error
	wif.TypeDefs.All()(func(_ string, td *wit.TypeDef) bool {
		if td.Name == nil {
			return true
		}
		tdName := *td.Name
		switch k := td.Kind.(type) {
		case *wit.Enum:
			// Skip typedefs imported via `use` — they belong to a different
			// interface and must not be re-declared here.
			if td.Owner != wif {
				return true
			}
			// Reuse the pre-registered stub if present so cross-interface
			// references share the same pointer; otherwise create fresh.
			var e *TypeEnum
			if existing, ok := typeDefIR[td]; ok {
				e = existing.(*TypeEnum)
			} else {
				e = &TypeEnum{Name: tdName, GoName: GoName(tdName, true)}
				typeDefIR[td] = e
			}
			for _, c := range k.Cases {
				e.Cases = append(e.Cases, EnumCase{
					Name:   c.Name,
					GoName: e.GoName + GoName(c.Name, true),
					Docs:   c.Docs.Contents,
				})
			}
			e.OwnerInterface = iface
			e.Docs = td.Docs.Contents
			iface.Enums = append(iface.Enums, e)
		case *wit.Flags:
			if td.Owner != wif {
				return true
			}
			var f *TypeFlags
			if existing, ok := typeDefIR[td]; ok {
				f = existing.(*TypeFlags)
			} else {
				f = &TypeFlags{Name: tdName, GoName: GoName(tdName, true)}
				typeDefIR[td] = f
			}
			for _, c := range k.Flags {
				f.Cases = append(f.Cases, FlagCase{
					Name:   c.Name,
					GoName: f.GoName + GoName(c.Name, true),
					Docs:   c.Docs.Contents,
				})
			}
			f.OwnerInterface = iface
			f.Docs = td.Docs.Contents
			iface.Flags = append(iface.Flags, f)
		case *wit.Record:
			if td.Owner != wif {
				return true
			}
			var r *TypeRecord
			if existing, ok := typeDefIR[td]; ok {
				r = existing.(*TypeRecord)
			} else {
				r = &TypeRecord{Name: tdName, GoName: GoName(tdName, true)}
				typeDefIR[td] = r
			}
			for _, fld := range k.Fields {
				ft, ferr := lowerType(fld.Type, typeDefIR)
				if ferr != nil {
					typeDefErr = fmt.Errorf("record %q field %q: %w", tdName, fld.Name, ferr)
					return false
				}
				r.Fields = append(r.Fields, RecordField{
					Name:   fld.Name,
					GoName: GoName(fld.Name, true),
					Type:   ft,
					Docs:   fld.Docs.Contents,
				})
			}
			r.OwnerInterface = iface
			r.Docs = td.Docs.Contents
			iface.Records = append(iface.Records, r)
		case *wit.Variant:
			if td.Owner != wif {
				return true
			}
			var vt *TypeVariant
			if existing, ok := typeDefIR[td]; ok {
				vt = existing.(*TypeVariant)
			} else {
				vt = &TypeVariant{Name: tdName, GoName: GoName(tdName, true)}
				typeDefIR[td] = vt
			}
			for _, c := range k.Cases {
				vc := VariantCase{
					Name:   c.Name,
					GoName: vt.GoName + GoName(c.Name, true),
					Docs:   c.Docs.Contents,
				}
				if c.Type != nil {
					payload, perr := lowerType(c.Type, typeDefIR)
					if perr != nil {
						typeDefErr = fmt.Errorf("variant %q case %q: %w", tdName, c.Name, perr)
						return false
					}
					vc.Payload = payload
				}
				vt.Cases = append(vt.Cases, vc)
			}
			vt.OwnerInterface = iface
			vt.Docs = td.Docs.Contents
			iface.Variants = append(iface.Variants, vt)
		case *wit.Resource:
			// *wit.Resource is a marker — its methods, ctor and statics
			// live on the interface's Functions list and are classified
			// by Function.Kind in the second pass.
			// The stub was pre-registered in globalTypeDefIR; reuse it
			// so cross-interface references share the same pointer.
			_ = k
			existing, ok := typeDefIR[td]
			var rt *TypeResource
			if ok {
				rt = existing.(*TypeResource)
			} else {
				goName := GoName(tdName, true)
				rt = &TypeResource{
					Name:     tdName,
					GoName:   goName,
					WrapName: goName,
					ImplName: goName + "Impl",
				}
				typeDefIR[td] = rt
			}
			// Only treat this resource as locally-defined if this interface
			// actually owns it. A typedef brought in via `use R.{r}` has
			// td.Owner pointing at the original interface; skip adding it to
			// iface.Resources so it remains an imported reference.
			if td.Owner != wif {
				return true
			}
			// If the resource's Go name collides with the interface's Go
			// name, suffix the resource with "Resource" to disambiguate.
			// E.g. the WIT interface "terminal-input" which contains a
			// resource also named "terminal-input" would otherwise produce
			// two `type TerminalInput interface` declarations in the same
			// package. We mutate the shared stub so cross-interface
			// references also pick up the new name.
			if rt.WrapName == iface.WrapName {
				rt.GoName = rt.WrapName + "Resource"
				rt.WrapName = rt.GoName
				rt.ImplName = rt.GoName + "Impl"
			}
			rt.Docs = td.Docs.Contents
			iface.Resources = append(iface.Resources, rt)
		}
		// Other Kinds (Variant, anonymous compounds) fall through:
		// anonymous compounds are lowered structurally on demand.
		return true
	})
	if typeDefErr != nil {
		return nil, typeDefErr
	}

	// Second pass: iterate Functions in deterministic order and lower
	// each signature, resolving typedef references against typeDefIR.
	type fn struct {
		name string
		f    *wit.Function
	}
	var fns []fn
	wif.Functions.All()(func(k string, v *wit.Function) bool {
		fns = append(fns, fn{k, v})
		return true
	})
	sort.Slice(fns, func(i, j int) bool { return fns[i].name < fns[j].name })

	for _, e := range fns {
		switch k := e.f.Kind.(type) {
		case *wit.Freestanding:
			f, err := lowerFunc(e.name, e.f, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("func %q: %w", e.name, err)
			}
			iface.Funcs = append(iface.Funcs, f)
		case *wit.Constructor:
			rt, err := resolveResourceTypeDef(k.Type, typeDefIR, "constructor", e.name)
			if err != nil {
				return nil, err
			}
			ctor, err := lowerCtor(e.f, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("ctor %q: %w", e.name, err)
			}
			rt.Ctor = ctor
		case *wit.Method:
			rt, err := resolveResourceTypeDef(k.Type, typeDefIR, "method", e.name)
			if err != nil {
				return nil, err
			}
			m, err := lowerMethod(e.f, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("method %q: %w", e.name, err)
			}
			rt.Methods = append(rt.Methods, m)
		case *wit.Static:
			rt, err := resolveResourceTypeDef(k.Type, typeDefIR, "static", e.name)
			if err != nil {
				return nil, err
			}
			s, err := lowerStatic(rt, e.f, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("static %q: %w", e.name, err)
			}
			rt.Statics = append(rt.Statics, s)
		default:
			return nil, fmt.Errorf("unknown function kind %T for %q", k, e.name)
		}
	}
	return iface, nil
}

// resolveResourceTypeDef looks up the *TypeResource for the resource that
// owns a constructor / method / static function. The wit AST stores this
// as a wit.Type (interface), but in practice it is a *wit.TypeDef
// referencing a *wit.Resource.
func resolveResourceTypeDef(t wit.Type, typeDefIR map[*wit.TypeDef]Type, kind, fname string) (*TypeResource, error) {
	td, ok := t.(*wit.TypeDef)
	if !ok {
		return nil, fmt.Errorf("%s %q: owner type is %T, not *wit.TypeDef", kind, fname, t)
	}
	ir, ok := typeDefIR[td]
	if !ok {
		// Try the typedef root in case of an alias.
		if root := td.Root(); root != td {
			ir, ok = typeDefIR[root]
		}
	}
	if !ok {
		return nil, fmt.Errorf("%s %q: owner typedef has no IR registration", kind, fname)
	}
	rt, ok := ir.(*TypeResource)
	if !ok {
		return nil, fmt.Errorf("%s %q: owner is %T, not *TypeResource", kind, fname, ir)
	}
	return rt, nil
}

// lowerCtor lowers a wit.Function classified as a constructor. The wit AST
// represents the constructor's params directly; the implicit own<R> result
// is recorded by the caller via rt.Ctor.
func lowerCtor(wfn *wit.Function, typeDefIR map[*wit.TypeDef]Type) (*ResourceCtor, error) {
	c := &ResourceCtor{Docs: wfn.Docs.Contents}
	for _, p := range wfn.Params {
		pt, err := lowerType(p.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("ctor param %q: %w", p.Name, err)
		}
		c.Params = append(c.Params, &Param{
			GoName: GoName(p.Name, false),
			Type:   pt,
		})
	}
	return c, nil
}

// lowerMethod lowers a wit.Function classified as a method. The wit AST
// includes the receiver as Params[0] (typically borrow<R>); we strip it
// because the Go method receiver is implicit.
func lowerMethod(wfn *wit.Function, typeDefIR map[*wit.TypeDef]Type) (ResourceMethod, error) {
	name := wfn.BaseName()
	m := ResourceMethod{
		Name:   name,
		GoName: GoName(name, true),
		Docs:   wfn.Docs.Contents,
	}
	params := wfn.Params
	if len(params) > 0 {
		// First param is the borrow<R> receiver; skip it.
		params = params[1:]
	}
	for _, p := range params {
		pt, err := lowerType(p.Type, typeDefIR)
		if err != nil {
			return m, fmt.Errorf("method param %q: %w", p.Name, err)
		}
		m.Params = append(m.Params, &Param{
			GoName: GoName(p.Name, false),
			Type:   pt,
		})
	}
	if len(wfn.Results) == 1 {
		rt, err := lowerType(wfn.Results[0].Type, typeDefIR)
		if err != nil {
			return m, fmt.Errorf("method result: %w", err)
		}
		m.Result = rt
	} else if len(wfn.Results) > 1 {
		return m, fmt.Errorf("method has %d results — multi-result methods not supported", len(wfn.Results))
	}
	return m, nil
}

// lowerStatic lowers a wit.Function classified as a static function on a
// resource. Static functions take no receiver.
func lowerStatic(rt *TypeResource, wfn *wit.Function, typeDefIR map[*wit.TypeDef]Type) (ResourceStatic, error) {
	name := wfn.BaseName()
	s := ResourceStatic{
		Name:   name,
		GoName: rt.GoName + GoName(name, true),
		Docs:   wfn.Docs.Contents,
	}
	for _, p := range wfn.Params {
		pt, err := lowerType(p.Type, typeDefIR)
		if err != nil {
			return s, fmt.Errorf("static param %q: %w", p.Name, err)
		}
		s.Params = append(s.Params, &Param{
			GoName: GoName(p.Name, false),
			Type:   pt,
		})
	}
	if len(wfn.Results) == 1 {
		rtype, err := lowerType(wfn.Results[0].Type, typeDefIR)
		if err != nil {
			return s, fmt.Errorf("static result: %w", err)
		}
		s.Result = rtype
	} else if len(wfn.Results) > 1 {
		return s, fmt.Errorf("static has %d results — multi-result statics not supported", len(wfn.Results))
	}
	return s, nil
}

func lowerFunc(name string, wfn *wit.Function, typeDefIR map[*wit.TypeDef]Type) (*Func, error) {
	if _, ok := wfn.Kind.(*wit.Freestanding); !ok {
		return nil, fmt.Errorf("non-freestanding function (kind %T) — not yet supported", wfn.Kind)
	}
	f := &Func{
		WitName: name,
		GoName:  GoName(name, true),
		Docs:    wfn.Docs.Contents,
	}
	for _, p := range wfn.Params {
		typ, err := lowerType(p.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("param %q: %w", p.Name, err)
		}
		f.Params = append(f.Params, &Param{
			GoName: GoName(p.Name, false),
			Type:   typ,
		})
	}
	switch len(wfn.Results) {
	case 0:
		// no result
	case 1:
		typ, err := lowerType(wfn.Results[0].Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		f.Result = typ
	default:
		return nil, fmt.Errorf("multiple results — not yet supported (wit-tools post-1.227 returns one max)")
	}
	return f, nil
}

// lowerType maps a wit.Type to the witgen IR. Primitives lower to Prim,
// string/list/tuple to TypeString/*TypeList/*TypeTuple. Anonymous compound
// types reach us wrapped in a *wit.TypeDef whose Kind carries the actual
// shape; we follow type-alias chains via Root before dispatching on Kind.
//
// For nominal type references (enum, flags declared at interface scope),
// typeDefIR provides pointer-identity lookup so each reference resolves
// to the SAME IR pointer stored in iface.Enums / iface.Flags.
func lowerType(t wit.Type, typeDefIR map[*wit.TypeDef]Type) (Type, error) {
	switch v := t.(type) {
	case wit.Bool:
		return PrimBool, nil
	case wit.U8:
		return PrimU8, nil
	case wit.U16:
		return PrimU16, nil
	case wit.U32:
		return PrimU32, nil
	case wit.U64:
		return PrimU64, nil
	case wit.S8:
		return PrimS8, nil
	case wit.S16:
		return PrimS16, nil
	case wit.S32:
		return PrimS32, nil
	case wit.S64:
		return PrimS64, nil
	case wit.F32:
		return PrimF32, nil
	case wit.F64:
		return PrimF64, nil
	case wit.Char:
		return PrimChar, nil
	case wit.String:
		return TypeString{}, nil
	case *wit.TypeDef:
		// Nominal-reference fast path: a named typedef previously
		// collected (enum, flags) resolves to the same IR pointer.
		if ir, ok := typeDefIR[v]; ok {
			return ir, nil
		}
		// Follow alias chain before kind dispatch. Check the root as
		// well, in case the reference is an alias to a nominal type.
		root := v.Root()
		if root != v {
			if ir, ok := typeDefIR[root]; ok {
				return ir, nil
			}
		}
		return lowerTypeDefKind(root.Kind, typeDefIR)
	}
	return nil, fmt.Errorf("type %T not yet supported", t)
}

func lowerTypeDefKind(k wit.TypeDefKind, typeDefIR map[*wit.TypeDef]Type) (Type, error) {
	switch v := k.(type) {
	case *wit.List:
		elem, err := lowerType(v.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("list elem: %w", err)
		}
		return &TypeList{Elem: elem}, nil
	case *wit.Tuple:
		fields := make([]Type, len(v.Types))
		for i, ft := range v.Types {
			f, err := lowerType(ft, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("tuple[%d]: %w", i, err)
			}
			fields[i] = f
		}
		return &TypeTuple{Fields: fields}, nil
	case *wit.Option:
		elem, err := lowerType(v.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("option elem: %w", err)
		}
		return &TypeOption{Elem: elem}, nil
	case *wit.Result:
		r := &TypeResult{}
		if v.OK != nil {
			ok, err := lowerType(v.OK, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("result ok: %w", err)
			}
			r.OK = ok
		}
		if v.Err != nil {
			e, err := lowerType(v.Err, typeDefIR)
			if err != nil {
				return nil, fmt.Errorf("result err: %w", err)
			}
			r.Err = e
		}
		return r, nil
	case *wit.Own:
		inner, err := lowerType(v.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("own<>: %w", err)
		}
		rt, ok := inner.(*TypeResource)
		if !ok {
			return nil, fmt.Errorf("own<> inner type is %T, expected *TypeResource", inner)
		}
		return &TypeOwn{Resource: rt}, nil
	case *wit.Borrow:
		inner, err := lowerType(v.Type, typeDefIR)
		if err != nil {
			return nil, fmt.Errorf("borrow<>: %w", err)
		}
		rt, ok := inner.(*TypeResource)
		if !ok {
			return nil, fmt.Errorf("borrow<> inner type is %T, expected *TypeResource", inner)
		}
		return &TypeBorrow{Resource: rt}, nil
	// Primitive types can appear as TypeDef kinds when the WIT declares a
	// named type alias to a primitive (e.g. `type duration = u64`). Lower
	// them the same way as their concrete counterparts in lowerType.
	case wit.Bool:
		return PrimBool, nil
	case wit.U8:
		return PrimU8, nil
	case wit.U16:
		return PrimU16, nil
	case wit.U32:
		return PrimU32, nil
	case wit.U64:
		return PrimU64, nil
	case wit.S8:
		return PrimS8, nil
	case wit.S16:
		return PrimS16, nil
	case wit.S32:
		return PrimS32, nil
	case wit.S64:
		return PrimS64, nil
	case wit.F32:
		return PrimF32, nil
	case wit.F64:
		return PrimF64, nil
	case wit.Char:
		return PrimChar, nil
	case wit.String:
		return TypeString{}, nil
	}
	return nil, fmt.Errorf("type def kind %T not yet supported", k)
}
