export const SUPPORTED_CLI_MANIFEST_FORMAT_VERSION = "1.0.0" as const;

export type CliDocumentationText = {
  readonly id: string;
  readonly canonicalEnglish: string;
};

export type CliDocumentation = {
  readonly formatVersion: "1.0.0";
  readonly itemId: string;
  readonly documentation: {
    readonly title: CliDocumentationText;
    readonly description: CliDocumentationText;
  };
  readonly examples: readonly unknown[];
  readonly visibility: "internal" | "public";
  readonly sourceHash: string;
};

export type CliLifecycle = {
  readonly formatVersion: "1.0.0";
  readonly itemId: string;
  readonly state: "active" | "deprecated" | "removed";
  readonly since: string;
  readonly deprecated?: string;
  readonly removed?: string;
  readonly successor?: {
    readonly targetItemId: string;
    readonly canonicalEnglish: string;
  };
};

export type CliInputReference = {
  readonly type: "argument" | "flag";
  readonly id: string;
};

export type CliArgument = {
  readonly id: string;
  readonly name: string;
  readonly position: number;
  readonly kind: "positional";
  readonly valueType: "string";
  readonly required: boolean;
  readonly minCardinality: number;
  readonly maxCardinality: number;
  readonly variadic: boolean;
  readonly enum: readonly string[];
  readonly pattern: string;
  readonly completion: "none" | "static" | "dynamic";
  readonly channels: readonly ("cli" | "config" | "env" | "stdin")[];
  readonly doubleDash: "none" | "terminates-flags";
};

export type CliFlag = {
  readonly id: string;
  readonly long: string;
  readonly shorthand: string;
  readonly aliases: readonly string[];
  readonly scope: "inherited" | "local" | "persistent";
  readonly valueType: "bool" | "int" | "int64" | "string" | "stringArray";
  readonly enum?: readonly string[];
  readonly required: boolean;
  readonly default: string;
  readonly changedDefault: boolean;
  readonly noOptionDefault: string;
  readonly repeatable: boolean;
  readonly normalization: "" | "lowercase" | "lowercase-trim" | "trim";
  readonly completion: "none" | "static" | "dynamic";
  readonly binding: string;
  readonly visibility: "hidden" | "visible";
  readonly lifecycle: CliLifecycle;
};

export type CliRelationship = {
  readonly id: string;
  readonly kind:
    | "at-least-one"
    | "conditional"
    | "conflict"
    | "mutually-exclusive"
    | "required-together";
  readonly participants: readonly CliInputReference[];
  readonly when?: CliInputReference;
};

export type CliCommand = {
  readonly id: string;
  readonly name: string;
  readonly path: string;
  readonly aliases: readonly string[];
  readonly documentation: CliDocumentation;
  readonly lifecycle: CliLifecycle;
  readonly visibility: "hidden" | "visible";
  readonly runnable: boolean;
  readonly usage: Readonly<{ line: string; example?: string }>;
  readonly arguments?: Readonly<Record<string, CliArgument>>;
  readonly flags?: Readonly<Record<string, CliFlag>>;
  readonly relationships?: Readonly<Record<string, CliRelationship>>;
  readonly [property: string]: unknown;
};

export type CliManifest = {
  readonly formatVersion: typeof SUPPORTED_CLI_MANIFEST_FORMAT_VERSION;
  readonly rootPath: string;
  readonly commands: Readonly<Record<string, CliCommand>>;
};

export type CliManifestDiagnosticCode =
  | "contradictory_inheritance"
  | "duplicate_id"
  | "invalid_cardinality"
  | "invalid_hierarchy"
  | "invalid_reference"
  | "invalid_type"
  | "invalid_value"
  | "missing_field"
  | "unstable_id";

export type CliManifestDiagnostic = {
  readonly code: CliManifestDiagnosticCode;
  readonly path: readonly (number | string)[];
  readonly message: string;
};

export type CliManifestLoadState =
  | { readonly status: "loading" }
  | {
      readonly status: "unsupported-version";
      readonly receivedVersion: string;
      readonly supportedVersions: readonly string[];
    }
  | {
      readonly status: "invalid-contract";
      readonly diagnostics: readonly CliManifestDiagnostic[];
    }
  | { readonly status: "empty"; readonly manifest: CliManifest }
  | { readonly status: "ready"; readonly manifest: CliManifest };
