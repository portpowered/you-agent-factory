# Align the Factory schema with canonical invocation interpolation

The canonical Factory JSON mapper intentionally accepts exact
`${parameter}` placeholders in enum-backed authored fields such as
`workers[].modelProvider`, and runtime validation verifies that each placeholder
references a compatible invocation-signature parameter. The generated Factory
JSON Schema still restricts that field to concrete provider enum values, so a
canonical runnable Factory such as `@you/fusion` cannot validate directly
against the schema.

Review whether the public Factory contract should model the exact-placeholder
form as an explicit `oneOf` alongside concrete enum values. The change should
keep ordinary provider values strict, reuse the canonical placeholder grammar,
regenerate all Factory schema projections, and add contract cases proving both
valid declared placeholders and invalid arbitrary strings. Once the schema and
mapper agree, remove the validation-only concrete-provider projection from
`internal/packagedfactorycatalog`.
