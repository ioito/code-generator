package generators

import (
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"k8s.io/gengo/args"
	"k8s.io/gengo/generator"
	"k8s.io/gengo/namer"
	"k8s.io/gengo/types"
	"k8s.io/klog"

	"yunion.io/x/pkg/util/sets"
	"yunion.io/x/pkg/utils"

	"yunion.io/x/code-generator/pkg/common"
)

const (
	tagName = "onecloud:model-api-gen"
	//tagPkgName           = "onecloud:model-api-gen-pkg"
	SModelBase           = "SModelBase"
	CloudCommonDBPackage = "yunion.io/x/onecloud/pkg/cloudcommon/db"
	CloudProviderPackage = "yunion.io/x/onecloud/pkg/cloudprovider"
)

func extractTag(comments []string) []string {
	return types.ExtractCommentTags("+", comments)[tagName]
}

func checkTag(comments []string, require ...string) bool {
	vals := types.ExtractCommentTags("+", comments)[tagName]
	if len(require) == 0 {
		return len(vals) == 1 && vals[0] == ""
	}
	return reflect.DeepEqual(vals, require)
}

/*func extractPkgTag(comments []string) []string {
	return types.ExtractCommentTags("+", comments)[tagPkgName]
}*/

var (
	APIsPackage              = "yunion.io/x/onecloud/pkg/apis"
	APIsCloudProviderPackage = filepath.Join(APIsPackage, "cloudprovider")
)

func GetInputOutputPackageMap(apisPkg string) map[string]string {
	ret := map[string]string{
		CloudCommonDBPackage: apisPkg,
		CloudProviderPackage: filepath.Join(apisPkg, "cloudprovider"),
	}
	return ret
}

// NameSystems returns the name system used by the generators in this package.
func NameSystems() namer.NameSystems {
	return namer.NameSystems{
		"public":  namer.NewPublicNamer(0),
		"private": namer.NewPrivateNamer(0),
		"raw":     namer.NewRawNamer("", nil),
	}
}

// DefaultNameSystem returns the default name system for ordering the types to be
// processed by the generators in this package.
func DefaultNameSystem() string {
	return "public"
}

// Packages makes the api-gen package definition.
func Packages(ctx *generator.Context, arguments *args.GeneratorArgs) generator.Packages {
	boilerplate, err := arguments.LoadGoBoilerplate()
	if err != nil {
		klog.Fatalf("Failed loading boilerplate: %v", err)
	}

	inputs := sets.NewString(ctx.Inputs...)
	packages := generator.Packages{}
	//header := append([]byte(fmt.Sprintf("// +build !%s\n\n", arguments.GeneratedBuildTag)), boilerplate...)

	for i := range inputs {
		pkg := ctx.Universe[i]
		if pkg == nil {
			// If the input had no Go files, for example
			continue
		}
		klog.Infof("Considering pkg %q", pkg.Path)
		//pkgPath := pkg.Path
		outPkgName := strings.Split(filepath.Base(arguments.OutputPackagePath), ".")[0]
		packages = append(packages,
			&generator.DefaultPackage{
				PackageName: outPkgName,
				PackagePath: arguments.OutputPackagePath,
				HeaderText:  boilerplate,
				GeneratorFunc: func(c *generator.Context) []generator.Generator {
					return []generator.Generator{
						// Always generate a "doc.go" file.
						// generator.DefaultGen{OptionalName: "doc"},
						// Generate api types by model.
						NewApiGen(arguments.OutputFileBaseName, pkg.Path, "", ctx.Order),
					}
				},
			})
	}
	return packages
}

type apiGen struct {
	generator.DefaultGen
	// sourcePackage is source package of input types
	sourcePackage string
	// modelTypes record all model types in source package
	modelTypes sets.String
	// modelDependTypes record all model required types
	modelDependTypes sets.String
	// isCommonDBPackage
	isCommonDBPackage bool

	imports            namer.ImportTracker
	needImportPackages sets.String
	apisPkg            string
}

func isCommonDBPackage(pkg string) bool {
	// pkg is yunion.io/x/onecloud/pkg/cloudcommon/db
	return strings.HasSuffix(pkg, CloudCommonDBPackage)
}

func defaultAPIsPkg(srcPkg string) string {
	yunionPrefix := "yunion.io/x/"
	parts := strings.Split(srcPkg, yunionPrefix)
	projectName := strings.Split(parts[1], "/")[0]
	return filepath.Join(yunionPrefix, projectName, "pkg", "apis")
}

func NewApiGen(sanitizedName, sourcePackage, apisPkg string, pkgTypes []*types.Type) generator.Generator {
	if apisPkg == "" {
		apisPkg = defaultAPIsPkg(sourcePackage)
	}
	gen := &apiGen{
		DefaultGen: generator.DefaultGen{
			OptionalName: sanitizedName,
		},
		sourcePackage:      sourcePackage,
		modelTypes:         sets.NewString(),
		modelDependTypes:   sets.NewString(),
		isCommonDBPackage:  isCommonDBPackage(sourcePackage),
		imports:            generator.NewImportTracker(),
		needImportPackages: sets.NewString(),
		apisPkg:            apisPkg,
	}
	gen.collectTypes(pkgTypes)
	klog.V(1).Infof("sets: %v\ndepsets: %v", gen.modelTypes.List(), gen.modelDependTypes.List())
	return gen
}

func (g *apiGen) Namers(c *generator.Context) namer.NameSystems {
	// Have the raw namer for this file track what it imports.
	return namer.NameSystems{
		"public": namer.NewPublicNamer(0),
		"raw":    namer.NewRawNamer("", g.imports),
	}
}

func (g *apiGen) GetInputOutputPackageMap() map[string]string {
	return GetInputOutputPackageMap(g.apisPkg)
}

func (g *apiGen) collectTypes(pkgTypes []*types.Type) {
	for _, t := range pkgTypes {
		if t.Kind != types.Struct {
			continue
		}
		if !g.inSourcePackage(t) {
			continue
		}
		if includeType(t) || g.isResourceModel(t) {
			g.modelTypes.Insert(t.String())
			g.addDependTypes(t, g.modelTypes, g.modelDependTypes)
		}
	}
}

// getPrimitiveType return the primitive type of Map, Slice, Pointer or Chan
func getPrimitiveType(t *types.Type) *types.Type {
	var compondKinds = sets.NewString(
		string(types.Map),
		string(types.Slice),
		string(types.Pointer),
		string(types.Chan))

	if !compondKinds.Has(string(t.Kind)) {
		return t
	}

	et := t.Elem
	return getPrimitiveType(et)
}

func isModelBase(t *types.Type) bool {
	return t.Name.Name == SModelBase
}

func (g *apiGen) addDependTypes(t *types.Type, out, dependOut sets.String) {
	if t.Kind == types.Alias {
		t = getPrimitiveType(underlyingType(t))
		out.Insert(t.String())
	}
	for _, m := range t.Members {
		switch m.Type.Kind {
		case types.Struct:
			if isModelBase(m.Type) {
				continue
			}
			if !g.inSourcePackage(m.Type) {
				dependOut.Insert(m.Type.String())
				continue
			}
			out.Insert(m.Type.String())
			g.addDependTypes(m.Type, out, dependOut)
		case types.Pointer:
			et := m.Type.Elem
			//if et.Kind == types.Struct {
			if !g.inSourcePackage(et) {
				dependOut.Insert(et.String())
				continue
			}
			out.Insert(et.String())
			g.addDependTypes(et, out, dependOut)
			//}
		case types.Alias:
			mt := m.Type
			umt := underlyingType(mt)
			umt = getPrimitiveType(umt)
			if !g.inSourcePackage(mt) && !g.inSourcePackage(umt) {
				dependOut.Insert(mt.String())
				continue
			}
			out.Insert(mt.String(), umt.String())
			// maybe bug?
			g.addDependTypes(umt, out, dependOut)
		}
	}
}

func includeType(t *types.Type) bool {
	vals := extractTag(t.CommentLines)
	if len(vals) != 0 {
		return true
	}
	return false
}

func (g *apiGen) Filter(c *generator.Context, t *types.Type) bool {
	if g.modelTypes.Has(t.String()) {
		return true
	}
	/*if g.inSourcePackage(t) && g.modelDependTypes.Has(t.String()) {
		return true
	}*/
	return false
}

func (g *apiGen) isResourceModel(t *types.Type) bool {
	return common.IsResourceModel(t, g.isCommonDBPackage)
}

func (g *apiGen) args(t *types.Type) interface{} {
	a := generator.Args{
		"type": t,
	}
	return a
}

func (g *apiGen) Imports(c *generator.Context) []string {
	lines := g.imports.ImportLines()
	lines = append(lines, g.needImportPackages.List()...)
	return lines
}

func (g *apiGen) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	klog.V(2).Infof("Generating api model for type %s", t.String())

	sw := generator.NewSnippetWriter(w, c, "$", "$")

	sw.Do(fmt.Sprintf("// %s is an autogenerated struct via %s.\n", t.Name.Name, t.Name.String()), nil)

	// 1. generate resource base output by model to pkg/apis/<pkg>/generated.model.go
	switch t.Kind {
	case types.Struct:
		g.generateStructType(t, sw)
	case types.Alias:
		g.generatorAliasType(t, sw)
	default:
		klog.Fatalf("Unsupported type %s", t.Kind)
	}
	return sw.Error()
}

func (g *apiGen) generateStructType(t *types.Type, sw *generator.SnippetWriter) {
	//klog.Errorf("for type %q", t.String())
	sw.Do("type $.type|public$ struct {\n", g.args(t))
	g.generateFor(t, sw)
	sw.Do("}\n", nil)
}

func (g *apiGen) generatorAliasType(t *types.Type, sw *generator.SnippetWriter) {
	sw.Do("type $.type|public$ ", g.args(t))
	ut := t.Underlying
	switch ut.Kind {
	case types.Slice:
		elem := ut.Elem
		content := "$.type|public$"
		if elem.Kind == types.Pointer {
			content = g.getPointerSourcePackageName(elem)
		}
		sw.Do(fmt.Sprintf("[]%s", content), g.args(elem))
	default:
		sw.Do("$.type|public$ ", g.args(t.Underlying))
	}
	sw.Do("\n", nil)
}

func underlyingType(t *types.Type) *types.Type {
	for t.Kind == types.Alias {
		t = t.Underlying
	}
	return t
}

func (g *apiGen) needCopy(t *types.Type) bool {
	tStr := t.String()
	return !g.modelTypes.Has(tStr) && g.modelDependTypes.Has(tStr)
}

func (g *apiGen) generateFor(t *types.Type, sw *generator.SnippetWriter) {
	for _, mem := range t.Members {
		mt := mem.Type
		if isModelBase(mt) {
			continue
		}

		var f func(types.Member, *generator.SnippetWriter)
		switch mt.Kind {
		case types.Builtin:
			f = g.doBuiltin
		case types.Struct:
			f = g.doStruct
		case types.Interface:
			f = g.doInterface
		case types.Alias:
			f = g.doAlias
		case types.Pointer:
			f = g.doPointer
		case types.Slice:
			f = g.doSlice
		default:
			klog.Fatalf("Hit an unsupported type %v.%s, kind is %s", t, mt.Name.Name, mt.Kind)
			//klog.Warningf("Hit an unsupported type %v.%s, kind is %s", t, mt.Name.Name, mt.Kind)
		}
		f(mem, sw)
	}
}

type Member struct {
	name     string
	jsonTags []string
	mType    string
	namer    string
	embedded bool
}

func NewMember(name string) *Member {
	return &Member{
		name:     name,
		jsonTags: make([]string, 0),
		namer:    "raw",
	}
}

// Type override types.Type raw type
func (m *Member) Type(mType string) *Member {
	m.mType = mType
	return m
}

func (m *Member) Name(name string) *Member {
	m.name = name
	return m
}

func (m *Member) Namer(namer string) *Member {
	m.namer = namer
	return m
}

func (m *Member) Embedded() *Member {
	m.embedded = true
	return m
}

func (m *Member) AddTag(tags ...string) *Member {
	tSets := sets.NewString(tags...)
	tSets.Insert(m.jsonTags...)
	m.jsonTags = tSets.List()
	return m
}

func (m *Member) NoTag() *Member {
	m.jsonTags = nil
	return m
}

func NewModelMember(name string, tags ...string) *Member {
	jName := utils.CamelSplit(name, "_")
	m := NewMember(name)
	return m.AddTag(jName)
}

func (m *Member) Do(sw *generator.SnippetWriter, args interface{}) {
	var (
		typePart string
		ret      string
	)
	namePart := m.name
	if m.mType != "" {
		typePart = m.mType
	} else {
		typePart = fmt.Sprintf("$.type|%s$", m.namer)
	}
	if m.embedded {
		ret = typePart
	} else {
		ret = fmt.Sprintf("%s %s", namePart, typePart)
	}
	if len(m.jsonTags) != 0 {
		ret = fmt.Sprintf("%s `json:\"%s\"`", ret, strings.Join(m.jsonTags, ","))
	}
	sw.Do(fmt.Sprintf("%s\n", ret), args)
}

func (g *apiGen) doBuiltin(m types.Member, sw *generator.SnippetWriter) {
	NewModelMember(m.Name).Do(sw, g.args(m.Type))
}

var (
	TypeMap = map[string]struct {
		Type     string
		JSONTags []string
	}{
		"TriState": {
			"*bool",
			[]string{"omitempty"},
		},
	}
)

func (g *apiGen) doAlias(member types.Member, sw *generator.SnippetWriter) {
	name := member.Name
	mt := member.Type
	if ct, ok := TypeMap[mt.Name.Name]; ok {
		m := NewModelMember(name).AddTag(ct.JSONTags...).Type(ct.Type)
		m.Do(sw, nil)
		return
	}
	ut := underlyingType(mt)
	NewModelMember(name).Do(sw, g.args(ut))
}

func (g *apiGen) doSlice(member types.Member, sw *generator.SnippetWriter) {
	klog.Fatalf("--slice not implement")
}

func (g *apiGen) doStruct(member types.Member, sw *generator.SnippetWriter) {
	mt := member.Type
	klog.V(5).Infof("doStruct for %s", mt.Name.String())
	//inPkg := g.inSourcePackage(member.Type)
	m := NewModelMember(member.Name)
	if member.Embedded {
		m.Embedded()
		m.NoTag()
	}
	if g.inSourcePackage(mt) {
		m.Namer("public")
	} else if outPkg, ok := g.GetInputOutputPackageMap()[mt.Name.Package]; ok {
		g.needImportPackages.Insert(outPkg)
		m.Type(fmt.Sprintf("%s.%s", filepath.Base(outPkg), mt.Name.Name))
	}
	m.Do(sw, g.args(mt))
}

func (g *apiGen) doInterface(m types.Member, sw *generator.SnippetWriter) {
	// model can't embedded interface
	if m.Embedded {
		klog.Fatalf("%s used as embedded interface", m.String())
	}
	NewModelMember(m.Name).Do(sw, g.args(m.Type))
}

func (g *apiGen) inSourcePackage(t *types.Type) bool {
	return common.InSourcePackage(t, g.sourcePackage)
}

func (g *apiGen) getPointerSourcePackageName(t *types.Type) string {
	elem := t.Elem
	if !g.inSourcePackage(elem) {
		klog.Fatalf("pointer's elem %q not in package %q", elem.Name.String(), g.sourcePackage)
	}
	return fmt.Sprintf("*%s", elem.Name.Name)
}

func (g *apiGen) doPointer(m types.Member, sw *generator.SnippetWriter) {
	t := m.Type
	mem := NewModelMember(m.Name)
	elem := m.Type.Elem
	if g.inSourcePackage(elem) {
		mem.Type(g.getPointerSourcePackageName(t))
	}
	if m.Embedded {
		mem.Embedded()
		mem.NoTag()
	}
	args := g.args(m.Type)
	mem.Do(sw, args)
}

type ResourceModel struct {
	t *types.Type
}

func NewModelByType(t *types.Type) *ResourceModel {
	return &ResourceModel{
		t: t,
	}
}
