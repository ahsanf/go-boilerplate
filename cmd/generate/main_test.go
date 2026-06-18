package main

import (
	"os"
	"path/filepath"
	"testing"
)

// --- naming helpers ---

func TestToSnakeCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Name", "name"},
		{"IsActive", "is_active"},
		{"CreatedAt", "created_at"},
		{"SortBy", "sort_by"},
		{"PlatformService", "platform_service"},
		{"URL", "u_r_l"},
	}
	for _, c := range cases {
		if got := toSnakeCase(c.in); got != c.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToKebabCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"IsActive", "is-active"},
		{"PlatformService", "platform-service"},
		{"Name", "name"},
	}
	for _, c := range cases {
		if got := toKebabCase(c.in); got != c.want {
			t.Errorf("toKebabCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"name", "name"},
		{"is_active", "isActive"},
		{"created_at", "createdAt"},
		{"total_items", "totalItems"},
		{"sort_by", "sortBy"},
	}
	for _, c := range cases {
		if got := toCamelCase(c.in); got != c.want {
			t.Errorf("toCamelCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNaivePlural(t *testing.T) {
	cases := []struct{ in, want string }{
		{"platform", "platforms"},
		{"product", "products"},
		{"class", "classes"},
		{"address", "addresses"},
	}
	for _, c := range cases {
		if got := naivePlural(c.in); got != c.want {
			t.Errorf("naivePlural(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- field parsing ---

func writeTemp(t *testing.T, src string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "entity.go")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestParseEntityFields_Basic(t *testing.T) {
	src := "package testpkg\n\n" +
		"import (\n\t\"time\"\n\t\"go.mongodb.org/mongo-driver/v2/bson\"\n)\n\n" +
		"type Product struct {\n" +
		"\tID        bson.ObjectID `bson:\"_id\"`\n" +
		"\tName      string        `bson:\"name\"`\n" +
		"\tPrice     float64       `bson:\"price\"`\n" +
		"\tIsActive  bool          `bson:\"is_active\"`\n" +
		"\tCreatedAt time.Time     `bson:\"created_at\"`\n" +
		"\tUpdatedAt time.Time     `bson:\"updated_at\"`\n" +
		"}\n"

	fields, err := parseEntityFields(writeTemp(t, src), "Product")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// _id, created_at, updated_at are system fields — must be excluded
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(fields), fields)
	}

	want := []struct {
		name     string
		bsonTag  string
		jsonTag  string
		respType string
		isTime   bool
	}{
		{"Name", "name", "name", "string", false},
		{"Price", "price", "price", "float64", false},
		{"IsActive", "is_active", "isActive", "bool", false},
	}

	for i, w := range want {
		f := fields[i]
		if f.Name != w.name {
			t.Errorf("fields[%d].Name = %q, want %q", i, f.Name, w.name)
		}
		if f.BsonTag != w.bsonTag {
			t.Errorf("fields[%d].BsonTag = %q, want %q", i, f.BsonTag, w.bsonTag)
		}
		if f.JsonTag != w.jsonTag {
			t.Errorf("fields[%d].JsonTag = %q, want %q", i, f.JsonTag, w.jsonTag)
		}
		if f.RespType != w.respType {
			t.Errorf("fields[%d].RespType = %q, want %q", i, f.RespType, w.respType)
		}
		if f.IsTime != w.isTime {
			t.Errorf("fields[%d].IsTime = %v, want %v", i, f.IsTime, w.isTime)
		}
		if f.ValidTag != "required" {
			t.Errorf("fields[%d].ValidTag = %q, want %q", i, f.ValidTag, "required")
		}
	}
}

func TestParseEntityFields_TimeType(t *testing.T) {
	src := "package testpkg\n\nimport \"time\"\n\n" +
		"type Event struct {\n" +
		"\tStartAt time.Time `bson:\"start_at\"`\n" +
		"}\n"

	fields, err := parseEntityFields(writeTemp(t, src), "Event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}

	f := fields[0]
	if !f.IsTime {
		t.Error("IsTime should be true for time.Time field")
	}
	if f.ReqType != "time.Time" {
		t.Errorf("ReqType = %q, want %q", f.ReqType, "time.Time")
	}
	if f.RespType != "string" {
		t.Errorf("RespType = %q, want %q (time.Time maps to string in response)", f.RespType, "string")
	}
	if f.JsonTag != "startAt" {
		t.Errorf("JsonTag = %q, want %q", f.JsonTag, "startAt")
	}
}

func TestParseEntityFields_FallbackBsonTag(t *testing.T) {
	// Field with no bson tag — should derive from field name
	src := "package testpkg\n\n" +
		"type Foo struct {\n" +
		"\tWorkunitId string\n" +
		"}\n"

	fields, err := parseEntityFields(writeTemp(t, src), "Foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}

	f := fields[0]
	if f.BsonTag != "workunit_id" {
		t.Errorf("BsonTag = %q, want %q (snake_case of field name)", f.BsonTag, "workunit_id")
	}
	if f.JsonTag != "workunitId" {
		t.Errorf("JsonTag = %q, want %q", f.JsonTag, "workunitId")
	}
}

func TestParseEntityFields_SystemFieldsExcluded(t *testing.T) {
	src := "package testpkg\n\nimport \"time\"\n\n" +
		"type Bare struct {\n" +
		"\tCreatedAt time.Time `bson:\"created_at\"`\n" +
		"\tUpdatedAt time.Time `bson:\"updated_at\"`\n" +
		"}\n"

	fields, err := parseEntityFields(writeTemp(t, src), "Bare")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("expected 0 fields after excluding system fields, got %d", len(fields))
	}
}

func TestParseEntityFields_StructNotFound(t *testing.T) {
	src := "package testpkg\n\ntype Foo struct{ Name string }\n"

	_, err := parseEntityFields(writeTemp(t, src), "Bar")
	if err == nil {
		t.Error("expected error when struct name not found in file")
	}
}

func TestParseEntityFields_InvalidFile(t *testing.T) {
	_, err := parseEntityFields("/nonexistent/path/entity.go", "Foo")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
