# Publish the rich CLI command manifest from the API package

The `@you-agent-factory/api/cli` export currently contains the reduced
`cli-command-identity/v1` inventory, while the canonical rich CLI manifest at
`contracts/cli/commands.json` uses format `1.0.0` and owns the input,
cardinality, inheritance, and relationship metadata required by downstream
consumers. A website or external package consumer cannot build authoritative
static controls from the published export without reaching back into repository
contract sources.

Align the package-generation boundary so the CLI export publishes the reviewed
rich manifest (or add an explicitly named rich-manifest export while retaining
the identity inventory for its current consumers). Keep the package manifest,
artifact hashes, isolated-consumer verification, and compatibility policy in
sync, and prove that an installed external consumer can resolve and validate the
rich command graph without repository-local paths.
