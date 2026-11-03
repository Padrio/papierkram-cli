// Package spec holds the Papierkram OpenAPI spec as patched by api/overlay.yaml.
package spec

// go generate runs the directives of this file top to bottom, and the order
// matters: the overlay step must write openapi.gen.json before any later
// directive (client codegen, dispatch table) reads it. Append new directives
// below, never above.

//go:generate go run ../../tools/applyoverlay ../../api/openapi.json ../../api/overlay.yaml openapi.gen.json
//go:generate go tool oapi-codegen -config ../api/gen/oapi-codegen.yaml openapi.gen.json
//go:generate go run ../../tools/gendispatch openapi.gen.json ../api/gen/dispatch.gen.go
