package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"time"
	"unicode"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// FieldInfo describes one non-system struct field.
type FieldInfo struct {
	Name         string // PascalCase Go name
	Type         string // Go type as written in the source
	BsonTag      string // bson/db field (column) name
	JsonTag      string // json field name (derived from bson/db tag)
	ValidTag     string // validate tag value (default "required")
	ReqType      string // type used in Request struct (same as Type)
	RespType     string // type used in Response struct (string for time.Time)
	IsTime       bool   // true when type is time.Time
	PGColumnType string // Postgres column type (only set when --db=postgres)
}

// TemplateData is passed into every .tmpl file.
type TemplateData struct {
	ModulePath   string
	Package      string
	Domain       string
	DomainSnake  string
	DomainKebab  string
	DomainPlural string
	Fields       []FieldInfo

	// Postgres-only: precomputed SQL fragments (empty for --db=mongo)
	PGInsertColumns      string
	PGInsertPlaceholders string
	PGUpdateSet          string
	PGUpdateWherePH      string
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

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 1 {
		return s
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		if len(p) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
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

// systemFieldTags are fields we skip when building Request/Response (they are
// always included via the template header/footer), keyed by --db value.
var systemFieldTags = map[string]map[string]bool{
	"mongo":    {"_id": true, "created_at": true, "updated_at": true},
	"postgres": {"id": true, "created_at": true, "updated_at": true},
}

// tagNameForDB is the struct tag the field parser reads column/field names from.
var tagNameForDB = map[string]string{
	"mongo":    "bson",
	"postgres": "db",
}

func parseEntityFields(filename, domainName, dbType string) ([]FieldInfo, error) {
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
			return extractFields(st, dbType), nil
		}
	}
	return nil, fmt.Errorf("struct %q not found in %s", domainName, filename)
}

func extractFields(st *ast.StructType, dbType string) []FieldInfo {
	tagName := tagNameForDB[dbType]
	systemTags := systemFieldTags[dbType]

	var fields []FieldInfo
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue // embedded field — skip
		}
		name := f.Names[0].Name
		typStr := typeExprString(f.Type)

		colTag := toSnakeCase(name)
		if f.Tag != nil {
			raw := f.Tag.Value[1 : len(f.Tag.Value)-1] // strip backticks
			if v := reflect.StructTag(raw).Get(tagName); v != "" {
				colTag = strings.Split(v, ",")[0]
			}
		}

		if systemTags[colTag] {
			continue
		}

		isTime := typStr == "time.Time"
		respType := typStr
		if isTime {
			respType = "string"
		}

		fields = append(fields, FieldInfo{
			Name:         name,
			Type:         typStr,
			BsonTag:      colTag,
			JsonTag:      toCamelCase(colTag),
			ValidTag:     "required",
			ReqType:      typStr,
			RespType:     respType,
			IsTime:       isTime,
			PGColumnType: pgColumnType(typStr),
		})
	}
	return fields
}

// pgColumnType maps a Go type (as written in the source struct) to a
// best-effort Postgres column type for the generated migration/schema comment.
func pgColumnType(goType string) string {
	switch goType {
	case "string":
		return "TEXT NOT NULL"
	case "bool":
		return "BOOLEAN NOT NULL"
	case "int", "int8", "int16", "int32", "uint", "uint8", "uint16", "uint32":
		return "INTEGER NOT NULL"
	case "int64", "uint64":
		return "BIGINT NOT NULL"
	case "float32", "float64":
		return "DOUBLE PRECISION NOT NULL"
	case "time.Time":
		return "TIMESTAMPTZ NOT NULL"
	default:
		return "TEXT NOT NULL -- TODO: verify column type for " + goType
	}
}

// --- Postgres SQL fragment helpers (column lists, placeholders, SET clause) ---

func pgInsertColumns(fields []FieldInfo) string {
	cols := make([]string, 0, len(fields)+2)
	for _, f := range fields {
		cols = append(cols, f.BsonTag)
	}
	cols = append(cols, "created_at", "updated_at")
	return strings.Join(cols, ", ")
}

func pgPlaceholders(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(ph, ", ")
}

func pgUpdateSet(fields []FieldInfo) string {
	sets := make([]string, 0, len(fields)+1)
	for i, f := range fields {
		sets = append(sets, fmt.Sprintf("%s = $%d", f.BsonTag, i+1))
	}
	sets = append(sets, fmt.Sprintf("updated_at = $%d", len(fields)+1))
	return strings.Join(sets, ", ")
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

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	out := buf.Bytes()
	if strings.HasSuffix(outPath, ".go") {
		if formatted, err := format.Source(out); err == nil {
			out = formatted
		}
		// on format error, fall through and write the unformatted source so the
		// generator's own compiler error points at a readable file.
	}

	return os.WriteFile(outPath, out, 0o644)
}

// wireInAppGo inserts the module import and wiring lines into app.go.
// Returns true if app.go was updated, false if skipped (not found / already wired).
func wireInAppGo(modulePath, pkg, domain, outputDir, clientExpr string) bool {
	const appPath = "cmd/server/main.go"
	raw, err := os.ReadFile(appPath)
	if err != nil {
		return false
	}

	importPath := fmt.Sprintf(`"%s/%s"`, modulePath, filepath.ToSlash(outputDir))
	src := string(raw)
	changed := false

	// 1. Add import after the last "go-boilerplate/ line in the import block.
	if !strings.Contains(src, importPath) {
		lines := strings.Split(src, "\n")
		lastIdx := -1
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), `"go-boilerplate/`) {
				lastIdx = i
			}
		}
		if lastIdx >= 0 {
			out := make([]string, 0, len(lines)+1)
			out = append(out, lines[:lastIdx+1]...)
			out = append(out, "\t"+importPath)
			out = append(out, lines[lastIdx+1:]...)
			src = strings.Join(out, "\n")
			changed = true
		}
	}

	// 2. Insert wiring after the sentinel comment (skip if already wired).
	const sentinel = "// --- Wire modules here ---"
	wireBlock := fmt.Sprintf(
		"\n\t%sRepo := %s.New%sRepository(%s)\n\t%sSvc  := %s.New%sService(%sRepo, utils.Log)\n\t%s.New%sHandler(app, %sSvc)",
		pkg, pkg, domain, clientExpr,
		pkg, pkg, domain, pkg,
		pkg, domain, pkg,
	)
	if strings.Contains(src, sentinel) && !strings.Contains(src, pkg+"Repo :=") {
		src = strings.Replace(src, sentinel, sentinel+wireBlock, 1)
		changed = true
	}

	if !changed {
		return false
	}

	if err := os.WriteFile(appPath, []byte(src), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update app.go: %v\n", err)
		return false
	}
	fmt.Println("wired    app.go")
	return true
}

// --- main ---

// dbClientExpr is the wiring expression for each supported --db value, e.g.
// `fooRepo := foo.NewFooRepository(<expr>)`.
var dbClientExpr = map[string]string{
	"mongo":    "configs.MongoClient",
	"postgres": "configs.PostgresPool",
}

func main() {
	domain := flag.String("domain", "", "Domain name in PascalCase, e.g. Product (required)")
	entityFile := flag.String("file", "", "Path to an existing Go entity file to parse struct fields from (optional)")
	outDir := flag.String("out", "", "Output directory (default: modules/<snake_domain>)")
	dbType := flag.String("db", "mongo", "Database driver: mongo | postgres")
	flag.Parse()

	if *domain == "" {
		fmt.Fprintln(os.Stderr, "error: --domain is required")
		flag.Usage()
		os.Exit(1)
	}

	if _, ok := dbClientExpr[*dbType]; !ok {
		fmt.Fprintf(os.Stderr, "error: --db must be one of: mongo, postgres (got %q)\n", *dbType)
		os.Exit(1)
	}

	domainSnake := toSnakeCase(*domain)
	domainKebab := toKebabCase(*domain)
	pkg := toPackageName(*domain)

	outputDir := *outDir
	if outputDir == "" {
		outputDir = filepath.Join("internal", "modules", domainSnake)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	var fields []FieldInfo
	if *entityFile != "" {
		var err error
		fields, err = parseEntityFields(*entityFile, *domain, *dbType)
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

	if *dbType == "postgres" {
		data.PGInsertColumns = pgInsertColumns(fields)
		data.PGInsertPlaceholders = pgPlaceholders(len(fields) + 2)
		data.PGUpdateSet = pgUpdateSet(fields)
		data.PGUpdateWherePH = fmt.Sprintf("$%d", len(fields)+2)
	}

	type outFile struct {
		tmpl string
		name string
	}

	var files []outFile
	if *dbType == "postgres" {
		files = []outFile{
			{"entity-postgres.tmpl", "entity.go"},
			{"repository-postgres.tmpl", "repository.go"},
			{"service-postgres.tmpl", "service.go"},
			{"handler.tmpl", "handler.go"},
		}
	} else {
		files = []outFile{
			{"entity.tmpl", "entity.go"},
			{"repository.tmpl", "repository.go"},
			{"service.tmpl", "service.go"},
			{"handler.tmpl", "handler.go"},
		}
	}

	for _, of := range files {
		outPath := filepath.Join(outputDir, of.name)
		if err := renderTemplate(of.tmpl, data, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("created  %s\n", outPath)
	}

	var migrationPath string
	if *dbType == "postgres" {
		var err error
		migrationPath, err = writeMigration(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write migration: %v\n", err)
		} else {
			fmt.Printf("created  %s\n", migrationPath)
		}
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

	clientExpr := dbClientExpr[*dbType]
	wired := wireInAppGo(data.ModulePath, pkg, *domain, outputDir, clientExpr)

	fmt.Println()
	fmt.Printf("✔  Module %q generated in %s (db=%s)\n", *domain, outputDir, *dbType)
	fmt.Println()
	fmt.Println("Next steps:")
	step := 1
	if !wired {
		fmt.Printf("  %d. Wire in app.go:\n", step)
		fmt.Printf("       %sRepo := %s.New%sRepository(%s)\n", pkg, pkg, *domain, clientExpr)
		fmt.Printf("       %sSvc  := %s.New%sService(%sRepo, utils.Log)\n", pkg, pkg, *domain, pkg)
		fmt.Printf("       %s.New%sHandler(app, %sSvc)\n", pkg, *domain, pkg)
		fmt.Printf("  %d. Add the import:  \"%s/%s\"\n", step+1, data.ModulePath, strings.ReplaceAll(outputDir, "\\", "/"))
		step += 2
	}
	if *dbType == "postgres" && migrationPath != "" {
		fmt.Printf("  %d. Run the migration against your Postgres database: psql $POSTGRES_URL -f %s\n", step, migrationPath)
		step++
	}
	fmt.Printf("  %d. Run: go mod tidy && swag init\n", step)
}

// writeMigration renders the Postgres CREATE TABLE migration into
// scripts/migrations/<timestamp>_create_<table>.sql.
func writeMigration(data TemplateData) (string, error) {
	const dir = "scripts/migrations"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_create_%s.sql", time.Now().Format("20060102150405"), data.DomainPlural)
	outPath := filepath.Join(dir, name)
	return outPath, renderTemplate("migration-postgres.tmpl", data, outPath)
}

