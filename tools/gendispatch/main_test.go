package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func param(typ, format string) *openapi3.Parameter {
	return &openapi3.Parameter{Name: "id", In: "path", Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{typ}, Format: format}}}
}

func TestGoTypeOnlyInt64(t *testing.T) {
	if typ, err := goType(param("integer", "int64")); err != nil || typ != "int64" {
		t.Fatalf("int64: %q %v", typ, err)
	}
	for _, p := range []*openapi3.Parameter{
		param("integer", "int32"),
		param("integer", ""),
		param("string", ""),
		{Name: "id", In: "path"},
	} {
		if typ, err := goType(p); err == nil || !strings.Contains(err.Error(), `"id"`) {
			t.Errorf("%+v: want generator error, got %q %v", p.Schema, typ, err)
		}
	}
}
