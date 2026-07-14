# Restricted OpenAPI 3.0.3 → JSON Schema Draft 2020-12 Converter Profile

This document defines the reviewed conversion profile implemented by
`internal/contractopenapiconverter`. The converter is build-time-only contract
tooling and must not be imported by API, config, service, CLI, worker, or
runtime packages.

Profile stories expand coverage in phases. This document records the cumulative
supported surface as each story lands.

## Profile stages

| Stage | Story | Coverage |
| --- | --- | --- |
| `core-shapes` | `interfaces-b15-factory-converter-001` | Primitives, objects, arrays, enums, required |
| `refs` | `interfaces-b15-factory-converter-002` | `#/components/schemas/...` → `#/$defs/...` |
| `composition-nullable` | `interfaces-b15-factory-converter-003` | `allOf` / `oneOf` / `anyOf`, OpenAPI `nullable` |
| `fail-closed` | `interfaces-b15-factory-converter-004` | Unsupported keywords, external refs, cycles, ambiguity |
| `staging` | `interfaces-b15-factory-converter-005` | Factory schema staging integration |

## Stage: `core-shapes`

`ConvertCoreSchema` accepts one OpenAPI 3.0.3 schema object with only the
keywords listed below. Every other keyword is rejected with diagnostic code
`openapi.convert.unsupported_keyword`.

### Supported keywords

| Keyword | Draft 2020-12 mapping |
| --- | --- |
| `type` | Copied verbatim. Supported values: `string`, `number`, `integer`, `boolean`, `object`, `array`. |
| `format` | Copied verbatim when `type` is a supported primitive (`string`, `number`, `integer`). |
| `enum` | Copied verbatim. Array element order is preserved. |
| `properties` | Copied as an object whose values are converted recursively with paths under `/properties/<name>`. |
| `required` | Copied verbatim. Property-name order is preserved. |
| `additionalProperties` | `false` and `true` are copied verbatim. Schema objects are converted recursively. |
| `items` | Converted recursively with path `/items`. |
| `description` | Copied verbatim. |
| `title` | Copied verbatim. |
| `default` | Copied verbatim when present on a schema that declares a supported `type` or `enum`. |
| `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` | Copied verbatim when `type` is `number` or `integer`. |
| `minLength`, `maxLength`, `pattern` | Copied verbatim when `type` is `string`. |
| `minItems`, `maxItems`, `uniqueItems` | Copied verbatim when `type` is `array`. |

### Explicit non-goals for `core-shapes`

The following are out of profile for this stage and must not appear in inputs
handled by `ConvertCoreSchema`:

- `$ref` and every other reference form
- `nullable`
- `allOf`, `oneOf`, `anyOf`, `not`, `if`, `then`, `else`
- `discriminator`
- `const`, `dependentSchemas`, `dependentRequired`, `patternProperties`,
  `propertyNames`, `unevaluatedProperties`, `unevaluatedItems`, `prefixItems`,
  `contains`, `minContains`, `maxContains`
- Vendor extensions (`x-*`)

Later profile stages document the exact Draft 2020-12 mapping for references,
composition, nullable forms, and the fail-closed rejection contract.

## Stage: `refs`

`ConvertRefsSchema` accepts one OpenAPI 3.0.3 root schema object plus the
`components.schemas` map that supplies referenced component bodies. The stage
extends `core-shapes` by allowing `$ref` values that target internal component
schemas and by materializing referenced components under `$defs`.

### Supported references

| OpenAPI reference | Draft 2020-12 outcome |
| --- | --- |
| `#/components/schemas/<Name>` | `{"$ref": "#/$defs/<Name>"}` with `<Name>` materialized once under `$defs` |

Rules:

- `<Name>` must be a single path segment with no `/` characters.
- `$ref` must be the only keyword on the schema object that carries it.
- Shared and transitive component references produce one `$defs` entry per
  component name.
- `$defs` object-key order is canonical and independent of discovery order
  because output is serialized with `contractjoiner.MarshalCanonicalJSON`.

### Reference rejection for `refs`

The following reference forms are rejected and must not be silently rewritten:

| Case | Diagnostic code |
| --- | --- |
| External URL, absolute path, repository-escaping, or other non-component refs | `openapi.convert.unsupported_reference` |
| Missing `components.schemas/<Name>` target | `openapi.convert.missing_component` |
| Component reference cycle | `openapi.convert.reference_cycle` |
| `$ref` combined with sibling keywords | `openapi.convert.unsupported_reference` |
| Empty or non-string `$ref` | `openapi.convert.invalid_reference` |

Story `interfaces-b15-factory-converter-004` expands the fail-closed
rejection suite; this stage owns the reference classifier for supported internal
component refs and the deterministic `$defs` materialization contract.

### Explicit non-goals for `refs`

The following remain out of profile for this stage:

- `nullable`
- `allOf`, `oneOf`, `anyOf`, `not`, `if`, `then`, `else`
- `discriminator`
- every keyword listed as non-goal for `core-shapes` except the supported
  internal component `$ref` form above

### Canonical output

Converted schema objects are serialized with
`contractjoiner.MarshalCanonicalJSON`:

- object keys are sorted recursively,
- indentation is two spaces,
- exactly one trailing newline is appended,
- repeated conversion of unchanged input is byte-identical.

Converted fragments do not receive `$schema` or `$id` metadata unless a later
staging story adds document-root envelope fields at the generation boundary.
