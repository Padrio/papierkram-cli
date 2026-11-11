module github.com/Padrio/papierkram-cli

go 1.26.0

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.7
	github.com/getkin/kin-openapi v0.153.0
	github.com/goccy/go-yaml v1.19.2
	github.com/itchyny/gojq v0.12.20
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/oapi-codegen/runtime v1.8.0
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/rogpeppe/go-internal v1.16.0
	// Pinned to v1.24.0: oapi-codegen v2.8.2 does not compile against newer
	// versions (they switched to go.yaml.in/yaml/v3). Bump only together with oapi-codegen.
	github.com/speakeasy-api/openapi v1.24.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.1.0
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/term v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.9 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.8.2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/speakeasy-api/jsonpath v0.6.3 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	go.yaml.in/yaml/v4 v4.0.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
)

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
