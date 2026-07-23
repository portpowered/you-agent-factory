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

export type CliInputValue =
  | Readonly<{ boolean: boolean }>
  | Readonly<{ int: number }>
  | Readonly<{ int64: number }>
  | Readonly<{ string: string }>
  | Readonly<{ stringArray: readonly string[] }>;

export type CliInputValueType =
  | "bool"
  | "int"
  | "int64"
  | "string"
  | "stringArray";

export type CliAcceptedSource =
  | "cli"
  | "environment"
  | "factory-signature-default"
  | "manifest-default"
  | "operator-config"
  | "stdin";

type CliArgumentBase = {
  readonly id: string;
  readonly name: string;
  readonly position: number;
  readonly kind: "positional";
  readonly valueType: CliInputValueType;
  readonly required: boolean;
  readonly minCardinality: number;
  readonly maxCardinality: number;
  readonly variadic: boolean;
  readonly enum: readonly string[];
  readonly pattern: string;
  readonly completion: "none" | "static" | "dynamic";
  readonly doubleDash: "none" | "terminates-flags";
  readonly defaultValue?: CliInputValue;
};

type CliLegacyArgument = {
  readonly channels: readonly ("cli" | "config" | "env" | "stdin")[];
  readonly scope?: never;
  readonly acceptedSources?: never;
  readonly handlerBindingId?: never;
  readonly visibility?: never;
  readonly lifecycle?: never;
};

type CliRichArgument = {
  readonly scope: "local";
  readonly acceptedSources: readonly CliAcceptedSource[];
  readonly handlerBindingId: string;
  readonly visibility: "hidden" | "visible";
  readonly lifecycle: CliLifecycle;
  readonly channels?: never;
};

export type CliArgument = CliArgumentBase &
  (CliLegacyArgument | CliRichArgument);

type CliFlagBase = {
  readonly id: string;
  readonly long: string;
  readonly shorthand: string;
  readonly aliases: readonly string[];
  readonly scope: "inherited" | "local" | "persistent";
  readonly valueType: CliInputValueType;
  readonly enum?: readonly string[];
  readonly required: boolean;
  readonly repeatable: boolean;
  readonly normalization: "" | "lowercase" | "lowercase-trim" | "trim";
  readonly completion: "none" | "static" | "dynamic";
  readonly visibility: "hidden" | "visible";
  readonly lifecycle: CliLifecycle;
  readonly inheritedFromInputId?: string;
};

type CliLegacyFlag = {
  readonly default: string;
  readonly changedDefault: boolean;
  readonly noOptionDefault: string;
  readonly binding: string;
  readonly kind?: never;
  readonly minCardinality?: never;
  readonly maxCardinality?: never;
  readonly defaultValue?: never;
  readonly noOptionDefaultValue?: never;
  readonly acceptedSources?: never;
  readonly handlerBindingId?: never;
};

type CliRichFlag = {
  readonly kind: "named";
  readonly minCardinality: number;
  readonly maxCardinality: number;
  readonly defaultValue?: CliInputValue;
  readonly noOptionDefaultValue?: CliInputValue;
  readonly acceptedSources: readonly CliAcceptedSource[];
  readonly handlerBindingId: string;
  readonly default?: never;
  readonly changedDefault?: never;
  readonly noOptionDefault?: never;
  readonly binding?: never;
};

export type CliFlag = CliFlagBase & (CliLegacyFlag | CliRichFlag);

export type CliRelationship = {
  readonly id: string;
  readonly kind:
    | "at-least-one"
    | "conditional"
    | "conflict"
    | "dependency"
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
