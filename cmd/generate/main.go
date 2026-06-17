package main

import (
	"embed"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// FieldInfo describes one non-system struct field.
type FieldInfo struct {
	Name     string // PascalCase Go name
	Type     string // Go type as written in the source
	BsonTag  string // bson field name
	JsonTag  string // json field name (derived from bson tag)
	ValidTag string // validate tag value (default "required")
	ReqType  string // type used in Request struct (same as Type)
	RespType string // type used in Response struct (string for time.Time)
	IsTime   bool   // true when type is time.Time
}

// TemplateData is passed into every .tmpl file.
type TemplateData struct {
	ModulePath  string
	Package     string
	Domain      string
	DomainSnake string
	DomainKebab string
	DomainPlural string
	Fields      []FieldInfo
}

// --- naming helpers ---

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func toKebabCase(s string) string {
	return strings.ReplaceAll(toSnakeCase(s), "_", "-")
}

func toPackageName(s string) string {
	return strings.ToLower(s)
}

// naivePlural appends "s" — good enough for a generator (user can rename).
func naivePlural(s string) string {
	if strings.HasSuffix(s, "s") {
		return s + "es"
	}
	return s + "s"
}

// --- go.mod module path reader ---

func readModulePath() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "go-boilerplate"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return "go-boilerplate"
}

// --- struct parser ---

// systemBsonTags are fields we skip when building Request/Response (they are
// always included via the template header/footer).
var systemBsonTags = map[string]bool{
	"_id":        true,
	"created_at": true,
	"updated_at": true,
}

func parseEntityFields(filename, domainName string) ([]FieldInfo, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != domainName {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			return extractFields(st), nil
		}
	}
	return nil, fmt.Errorf("struct %q not found in %s", domainName, filename)
}

func extractFields(st *ast.StructType) []FieldInfo {
	var fields []FieldInfo
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded field — skip
		}
		name := f.Names[0].Name
		typStr := typeExprString(f.Type)

		bsonTag := toSnakeCase(name)
		if f.Tag != nil {
			raw := f.Tag.Value[1 : len(f.Tag.Value)-1] // strip backticks
			if v := reflect.StructTag(raw).Get("bson"); v != "" {
				bsonTag = strings.Split(v, ",")[0]
			}
		}

		if systemBsonTags[bsonTag] {
			continue
		}

		isTime := typStr == "time.Time"
		respType := typStr
		if isTime {
			respType = "string"
		}

		fields = append(fields, FieldInfo{
			Name:     name,
			Type:     typStr,
			BsonTag:  bsonTag,
			JsonTag:  bsonTag, // bson snake_case matches json snake_case
			ValidTag: "required",
			ReqType:  typStr,
			RespType: respType,
			IsTime:   isTime,
		})
	}
	return fields
}

func typeExprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeExprString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeExprString(t.X)
	case *ast.ArrayType:
		return "[]" + typeExprString(t.Elt)
	case *ast.MapType:
		return "map[" + typeExprString(t.Key) + "]" + typeExprString(t.Value)
	default:
		return "interface{}"
	}
}

// --- file generation ---

func renderTemplate(tmplName string, data TemplateData, outPath string) error {
	raw, err := templateFS.ReadFile("templates/" + tmplName)
	if err != nil {
		return err
	}

	tmpl, err := template.New(tmplName).Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("template parse error (%s): %w", tmplName, err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// --- main ---

func main() {
	domain := flag.String("domain", "", "Domain name in PascalCase, e.g. Product (required)")
	entityFile := flag.String("file", "", "Path to an existing Go entity file to parse struct fields from (optional)")
	outDir := flag.String("out", "", "Output directory (default: modules/<snake_domain>)")
	flag.Parse()

	if *domain == "" {
		fmt.Fprintln(os.Stderr, "error: --domain is required")
		flag.Usage()
		os.Exit(1)
	}

	domainSnake := toSnakeCase(*domain)
	domainKebab := toKebabCase(*domain)
	pkg := toPackageName(*domain)

	outputDir := *outDir
	if outputDir == "" {
		outputDir = filepath.Join("modules", domainSnake)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	var fields []FieldInfo
	if *entityFile != "" {
		var err error
		fields, err = parseEntityFields(*entityFile, *domain)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not parse fields from %s: %v\n", *entityFile, err)
			fmt.Fprintln(os.Stderr, "generating with empty fields — fill in entity.go manually")
		}
	}

	data := TemplateData{
		ModulePath:   readModulePath(),
		Package:      pkg,
		Domain:       *domain,
		DomainSnake:  domainSnake,
		DomainKebab:  domainKebab,
		DomainPlural: naivePlural(domainSnake),
		Fields:       fields,
	}

	type outFile struct {
		tmpl string
		name string
	}

	files := []outFile{
		{"entity.tmpl", "entity.go"},
		{"repository.tmpl", "repository.go"},
		{"service.tmpl", "service.go"},
		{"handler.tmpl", "handler.go"},
	}

	for _, of := range files {
		outPath := filepath.Join(outputDir, of.name)
		if err := renderTemplate(of.tmpl, data, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("created  %s\n", outPath)
	}

	// Remove the source entity file now that fields have been extracted and
	// entity.go has been generated from the template.
	if *entityFile != "" {
		if err := os.Remove(*entityFile); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: could not remove source file %s: %v\n", *entityFile, err)
		} else {
			fmt.Printf("removed  %s (fields extracted)\n", *entityFile)
		}
	}

	fmt.Println()
	fmt.Printf("✔  Module %q generated in %s\n", *domain, outputDir)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Wire in app.go:\n")
	fmt.Printf("       %sRepo := %s.New%sRepository(db)\n", pkg, pkg, *domain)
	fmt.Printf("       %sSvc  := %s.New%sService(%sRepo, utils.Logger)\n", pkg, pkg, *domain, pkg)
	fmt.Printf("       %s.New%sHandler(app, %sSvc)\n", pkg, *domain, pkg)
	fmt.Printf("  2. Add the import:  \"%s/%s\"\n", data.ModulePath, strings.ReplaceAll(outputDir, "\\", "/"))
	fmt.Println("  3. Run: go mod tidy && swag init")
}

