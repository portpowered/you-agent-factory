/**
 * Normalization-only factory contract boundary.
 *
 * `normalizeFactoryDefinition` and `isCanonicalFactoryDefinition` shape payloads against
 * the generated OpenAPI `Factory` schema. This folder performs no HTTP and does not
 * import `transport.ts`. Session factory GET/PUT lives under `session-factory/`; editor
 * and import adapters call normalization here then delegate network I/O elsewhere.
 */
export * from "./api";
export * from "./preserve-bundled-files";
