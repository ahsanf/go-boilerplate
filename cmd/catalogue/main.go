// cmd/catalogue/main.go
//
// Scans internal/modules/ and upserts per-domain sections into
// docs/data-catalogue.md and docs/service-catalogue.md.
//
// Usage:
//
//	go run cmd/catalogue/main.go                         # update both catalogues
//	go run cmd/catalogue/main.go --type data             # data catalogue only
//	go run cmd/catalogue/main.go --type service          # service catalogue only
//	go run cmd/catalogue/main.go --modules internal/modules --out docs
//	go run cmd/catalogue/main.go --dry-run               # print to stdout, no writes
//
// The command is idempotent: run it whenever you add or change a module.
// Sections bounded by <!-- gen:begin:Name --> / <!-- gen:end:Name --> are
// regenerated; everything else in the file is left untouched.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ── data types ────────────────────────────────────────────────────────────────

// FieldMeta describes one non-system field from an entity struct.
type FieldMeta struct {
	Name    string // PascalCase Go name
	GoType  string // Go type as written in source
	BsonTag string // bson field name (snake_case)
}

// RouteMeta describes one HTTP route extracted from Swagger annotations.
type RouteMeta struct {
	Method  string
	Path    string
	Summary string
}

// ModuleMeta aggregates everything discovered about one domain module.
type ModuleMeta struct {
	Name       string // PascalCase, e.g. "Product"
	Package    string // lowercase dir name, e.g. "product"
	Collection string // MongoDB collection name, e.g. "products"
	Fields     []FieldMeta
	Routes     []RouteMeta
}

// ── scanner ───────────────────────────────────────────────────────────────────

// scanModules walks modulesDir and builds a ModuleMeta for every subdirectory
// except "common".
func scanModules(modulesDir string) ([]ModuleMeta, error) {
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", modulesDir, err)
	}

	var out []ModuleMeta
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "common" {
			continue
		}
		dir := filepath.Join(modulesDir, e.Name())
		m, err := parseModule(dir, e.Name())
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", dir, err)
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func parseModule(dir, pkg string) (ModuleMeta, error) {
	m := ModuleMeta{
		Name:    toPascalCase(pkg),
		Package: pkg,
	}

	entityPath := filepath.Join(dir, "entity.go")
	if _, err := os.Stat(entityPath); err == nil {
		if fields, err := parseEntityFields(entityPath, m.Name); err == nil {
			m.Fields = fields
		}
	}

	repoPath := filepath.Join(dir, "repository.go")
	if _, err := os.Stat(repoPath); err == nil {
		m.Collection = parseCollectionConst(repoPath)
	}
	if m.Collection == "" {
		m.Collection = naivePlural(toSnakeCase(pkg))
	}

	handlerPath := filepath.Join(dir, "handler.go")
	if _, err := os.Stat(handlerPath); err == nil {
		m.Routes = parseRoutes(handlerPath)
	}

	return m, nil
}

// ── entity parser (go/ast) ────────────────────────────────────────────────────

var systemBsonTags = map[string]bool{
	"_id": true, "created_at": true, "updated_at": true,
}

func parseEntityFields(filename, domainName string) ([]FieldMeta, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
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

func extractFields(st *ast.StructType) []FieldMeta {
	var fields []FieldMeta
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue
		}
		name := f.Names[0].Name
		typStr := typeExprString(f.Type)
		bsonTag := toSnakeCase(name)
		if f.Tag != nil {
			raw := f.Tag.Value[1 : len(f.Tag.Value)-1] // strip backticks
			if v := parseStructTag(raw, "bson"); v != "" {
				bsonTag = strings.Split(v, ",")[0]
			}
		}
		if systemBsonTags[bsonTag] {
			continue
		}
		fields = append(fields, FieldMeta{Name: name, GoType: typStr, BsonTag: bsonTag})
	}
	return fields
}

// parseStructTag is a minimal struct tag value extractor (avoids reflect import).
func parseStructTag(tag, key string) string {
	for tag != "" {
		// skip spaces
		for len(tag) > 0 && tag[0] == ' ' {
			tag = tag[1:]
		}
		if tag == "" {
			break
		}
		// read key
		i := 0
		for i < len(tag) && tag[i] != ':' && tag[i] != ' ' {
			i++
		}
		if i >= len(tag) || tag[i] != ':' || i+1 >= len(tag) || tag[i+1] != '"' {
			break
		}
		k := tag[:i]
		tag = tag[i+2:] // skip `:"`
		// read value (until closing unescaped `"`)
		j := 0
		for j < len(tag) && tag[j] != '"' {
			if tag[j] == '\\' {
				j++
			}
			j++
		}
		v := tag[:j]
		if j < len(tag) {
			tag = tag[j+1:]
		} else {
			tag = ""
		}
		if k == key {
			return v
		}
	}
	return ""
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
		return "any"
	}
}

// ── repository parser ─────────────────────────────────────────────────────────

var reCollection = regexp.MustCompile(`collectionName\s*=\s*"([^"]+)"`)

func parseCollectionConst(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if m := reCollection.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// ── handler / route parser ────────────────────────────────────────────────────

var (
	reSummary = regexp.MustCompile(`@Summary\s+(.+)`)
	reRouter  = regexp.MustCompile(`@Router\s+(\S+)\s+\[(\w+)\]`)
)

// parseRoutes extracts routes from Swagger @Router + @Summary annotations.
func parseRoutes(path string) []RouteMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var routes []RouteMeta
	var pendingSummary string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := reSummary.FindStringSubmatch(line); m != nil {
			pendingSummary = strings.TrimSpace(m[1])
		}
		if m := reRouter.FindStringSubmatch(line); m != nil {
			routes = append(routes, RouteMeta{
				Path:    m[1],
				Method:  strings.ToUpper(m[2]),
				Summary: pendingSummary,
			})
			pendingSummary = ""
		}
	}
	return routes
}

// ── section renderers ─────────────────────────────────────────────────────────

func genBegin(name string) string { return fmt.Sprintf("<!-- gen:begin:%s -->", name) }
func genEnd(name string) string   { return fmt.Sprintf("<!-- gen:end:%s -->", name) }

func renderDataSection(idx int, m ModuleMeta) string {
	var sb strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&sb, format, args...) }

	w("%s\n", genBegin(m.Name))
	w("### 2.%d Entity: `%s`\n\n", idx, m.Name)

	w("| Field | Description |\n")
	w("|---|---|\n")
	w("| **Name** | %s |\n", m.Name)
	w("| **Collection** | `%s` |\n", m.Collection)
	w("| **Storage** | MongoDB |\n")
	w("| **Owner** | |\n")
	w("| **Sensitivity** | Internal |\n")
	w("| **PII** | No |\n")
	w("| **Retention** | |\n")
	w("| **Access Control** | |\n")
	w("| **Related Entities** | |\n\n")

	w("#### Attributes\n\n")
	w("| Attribute | Type | Nullable | Description |\n")
	w("|---|---|---|---|\n")
	w("| _id | bson.ObjectID | No | Primary key |\n")
	for _, f := range m.Fields {
		w("| %s | %s | No | |\n", f.BsonTag, f.GoType)
	}
	w("| created_at | time.Time | No | Record creation time |\n")
	w("| updated_at | time.Time | No | Last update time |\n")
	w("\n")
	w("%s\n", genEnd(m.Name))
	return sb.String()
}

func renderServiceSection(idx int, m ModuleMeta) string {
	var sb strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&sb, format, args...) }

	w("%s\n", genBegin(m.Name))
	w("### 2.%d Service: `%s`\n\n", idx, m.Name)

	w("| Field | Description |\n")
	w("|---|---|\n")
	w("| **Name** | %s |\n", m.Name)
	w("| **Purpose** | |\n")
	w("| **Owner** | |\n")
	w("| **Tech Stack** | Go, Fiber v2, MongoDB |\n")
	w("| **Type** | API |\n")
	w("| **Collection** | `%s` |\n\n", m.Collection)

	if len(m.Routes) > 0 {
		w("#### API Surface\n\n")
		w("| Endpoint | Method | Summary |\n")
		w("|---|---|---|\n")
		for _, r := range m.Routes {
			w("| `%s` | %s | %s |\n", r.Path, r.Method, r.Summary)
		}
		w("\n")
	}

	w("#### Dependencies\n\n")
	w("| Dependency | Type | Required? | Notes |\n")
	w("|---|---|---|---|\n")
	w("| MongoDB | Database | Yes | Collection: `%s` |\n", m.Collection)
	w("| Redis | Cache | No | |\n")
	w("\n")
	w("%s\n", genEnd(m.Name))
	return sb.String()
}

// ── upsert engine ─────────────────────────────────────────────────────────────

// genBlockRe matches a full gen:begin/end block including the trailing newline.
// Blocks never nest, so a non-greedy .*? between any begin/end pair is safe.
var genBlockRe = regexp.MustCompile(`(?s)<!-- gen:begin:\w+ -->.*?<!-- gen:end:\w+ -->\n?`)

// upsertDoc creates or updates a catalogue file.
//
// Strategy:
//  1. Strip all existing gen blocks from the document.
//  2. Normalise extra blank lines left behind.
//  3. Locate the ## 2. section and the next ## section (or EOF).
//  4. Insert all freshly rendered blocks between them.
//  5. Stamp "Last updated" date.
func upsertDoc(
	outPath string,
	modules []ModuleMeta,
	freshHeader, freshFooter string,
	renderFn func(int, ModuleMeta) string,
	dryRun bool,
) error {
	var content string
	if data, err := os.ReadFile(outPath); err == nil {
		content = string(data)
	} else {
		// File doesn't exist yet — build it from scratch.
		content = freshHeader + freshFooter
	}

	// Remove existing gen blocks.
	content = genBlockRe.ReplaceAllString(content, "")

	// Collapse runs of 3+ blank lines down to 2.
	multiBlankRe := regexp.MustCompile(`\n{3,}`)
	content = multiBlankRe.ReplaceAllString(content, "\n\n")

	// Find where to inject: between "## 2." and the next "## [3-9]." (or EOF).
	sec2Re := regexp.MustCompile(`(?m)^## 2\.`)
	sec3Re := regexp.MustCompile(`(?m)^## [3-9]\.`)

	sec2Loc := sec2Re.FindStringIndex(content)
	sec3Loc := sec3Re.FindStringIndex(content)

	var insertAt int
	switch {
	case sec2Loc == nil:
		insertAt = len(content)
	case sec3Loc == nil:
		insertAt = len(content)
	default:
		// Insert right before the "---" separator that precedes ## 3., if present.
		sep := "\n---\n"
		if idx := strings.LastIndex(content[:sec3Loc[0]], sep); idx > sec2Loc[0] {
			insertAt = idx + 1 // keep the leading \n, replace from the ---
		} else {
			insertAt = sec3Loc[0]
		}
	}

	// Build replacement block.
	var inject strings.Builder
	inject.WriteByte('\n')
	for i, m := range modules {
		inject.WriteString(renderFn(i+1, m))
		inject.WriteByte('\n')
	}

	content = content[:insertAt] + inject.String() + content[insertAt:]

	// Stamp "Last updated" date.
	today := time.Now().Format("2006-01-02")
	lastUpdatedRe := regexp.MustCompile(`(- Last updated:\s*).*`)
	content = lastUpdatedRe.ReplaceAllString(content, "${1}"+today)

	if dryRun {
		fmt.Println(content)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(content), 0o644)
}

// ── catalogue file templates ──────────────────────────────────────────────────

const dataCatalogueHeader = `# Data Catalogue

> **Owner:** Tech Lead / Data Owner
> **Required in every project.**

---

## 1. Overview

This document lists all data entities, their sources, ownership, sensitivity, and lifecycle rules.

### 1.1 Project
- Project name:
- Last updated:
- Owner:

### 1.2 Data Principles
- Data is treated as an asset.
- Sensitive data is classified and protected.
- Data retention follows legal and organizational policies.

---

## 2. Data Entities

`

const dataCatalogueFooter = `
---

## 3. Data Flow

Describe how data moves through the system.

` + "```" + `text
[Source] → [Service/API] → [MongoDB] → [Consumer]
` + "```" + `

Attach a data-flow diagram to ` + "`docs/dataflow-diagrams/`." + `

---

## 4. Data Classification

| Classification | Description | Examples | Handling |
|---|---|---|---|
| Public | Safe to disclose | Public landing page content | Standard controls |
| Internal | Business use only | Analytics aggregates | Limited access |
| Confidential | Sensitive business or user data | Email, phone number | Encryption, access logs |
| Restricted | Highly sensitive | Passwords, tokens, government IDs | Encryption, strict need-to-know |

---

## 5. Compliance & Retention

| Regulation / Policy | Requirement | Affected Data |
|---|---|---|
| GDPR / PDP | Right to erasure | User PII |
| Organization policy | Logs retained 90 days | Application logs |

---

## 6. Data Quality

- Validation rules:
- Cleansing procedures:
- Monitoring:

---

## 7. Backups & Recovery

| Data Store | Backup Frequency | Retention | Recovery Procedure |
|---|---|---|---|
| MongoDB | Daily | 30 days | Document in runbook |
`

const serviceCatalogueHeader = `# Service Catalogue

> **Owner:** Tech Lead
> **Required in every project.**

---

## 1. Overview

This document lists every service, its responsibilities, dependencies, ownership, and operational details.

### 1.1 Project
- Project name:
- Last updated:
- Owner:

### 1.2 Architecture Notes
- Monolith / microservices / serverless?
- Primary cloud / hosting platform:
- Link to architecture diagram in ` + "`docs/dataflow-diagrams/`." + `

---

## 2. Services

`

const serviceCatalogueFooter = `
---

## 3. Service Interaction Map

| Source Service | Target Service | Protocol | Purpose |
|---|---|---|---|
| | | HTTPS | |

---

## 4. External Integrations

| Integration | Purpose | Owner | Contact |
|---|---|---|---|
| Firebase Auth | Identity / token verification | Google | |
| Google PubSub | Async messaging | Google | |

---

## 5. On-Call & Escalation

| Severity | Response Time | Escalation Path |
|---|---|---|
| P1 | 15 minutes | Tech Lead → Engineering Manager |
| P2 | 1 hour | On-call engineer → Tech Lead |
| P3 | 1 business day | Create ticket |
`

// ── naming helpers ────────────────────────────────────────────────────────────

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

func toPascalCase(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if len(part) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

func naivePlural(s string) string {
	if strings.HasSuffix(s, "s") {
		return s + "es"
	}
	return s + "s"
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	modulesDir := flag.String("modules", filepath.Join("internal", "modules"), "Path to modules directory to scan")
	outDir := flag.String("out", "docs", "Output directory for catalogue files")
	genType := flag.String("type", "all", `What to generate: "data", "service", or "all"`)
	dryRun := flag.Bool("dry-run", false, "Print output to stdout instead of writing files")
	flag.Parse()

	modules, err := scanModules(*modulesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning modules: %v\n", err)
		os.Exit(1)
	}
	if len(modules) == 0 {
		fmt.Fprintln(os.Stderr, "no domain modules found in", *modulesDir)
		os.Exit(0)
	}

	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = m.Name
	}
	fmt.Printf("found %d module(s): %s\n", len(modules), strings.Join(names, ", "))

	if *genType == "data" || *genType == "all" {
		outPath := filepath.Join(*outDir, "data-catalogue.md")
		if err := upsertDoc(outPath, modules, dataCatalogueHeader, dataCatalogueFooter, renderDataSection, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if !*dryRun {
			fmt.Println("updated  " + outPath)
		}
	}

	if *genType == "service" || *genType == "all" {
		outPath := filepath.Join(*outDir, "service-catalogue.md")
		if err := upsertDoc(outPath, modules, serviceCatalogueHeader, serviceCatalogueFooter, renderServiceSection, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if !*dryRun {
			fmt.Println("updated  " + outPath)
		}
	}
}
