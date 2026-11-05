package infra

import _ "embed"

// schemaFileName is the JSON Schema saved next to the configs, which they
// point to with "$schema" so editors complete and check them.
const schemaFileName = "weaver.schema.json"

//go:embed weaver.schema.json
var schema []byte
