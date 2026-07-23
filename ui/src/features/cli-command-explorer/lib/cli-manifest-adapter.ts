import Ajv2020, { type ErrorObject } from "ajv/dist/2020.js";
import commandManifestSchema from "../../../../../contracts/cli/command-manifest.schema.json" with {
  type: "json",
};
import deprecationsSchema from "../../../../../contracts/common/deprecations.schema.json" with {
  type: "json",
};
import documentationSchema from "../../../../../contracts/common/documentation.schema.json" with {
  type: "json",
};
import { publishedCliManifestArtifact } from "../../../api/cli/published-cli-manifest";
import {
  type CliManifestMessages,
  getCliManifestMessages,
} from "../messages/cli-manifest";
import { validateCliInputSemantics } from "./cli-input-semantics";
import {
  type CliCommand,
  type CliFlag,
  type CliManifest,
  type CliManifestDiagnostic,
  type CliManifestDiagnosticCode,
  type CliManifestLoadState,
  SUPPORTED_CLI_MANIFEST_FORMAT_VERSION,
} from "./cli-manifest-types";

type JsonRecord = Record<string, unknown>;
type Path = readonly (number | string)[];

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
ajv.addSchema(documentationSchema).addSchema(deprecationsSchema);
const validateManifestShape = ajv.compile(commandManifestSchema);

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function pointerSegments(pointer: string): (number | string)[] {
  if (pointer.length === 0) return [];
  return pointer
    .slice(1)
    .split("/")
    .map((segment) => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
    .map((segment) =>
      /^(0|[1-9]\d*)$/.test(segment) ? Number(segment) : segment,
    );
}

function schemaIssuePath(error: ErrorObject): Path {
  const path = pointerSegments(error.instancePath);
  if (error.keyword === "required")
    return [...path, error.params.missingProperty];
  if (error.keyword === "additionalProperties") {
    return [...path, error.params.additionalProperty];
  }
  return path;
}

function schemaIssueCode(error: ErrorObject): CliManifestDiagnosticCode {
  const path = schemaIssuePath(error).join(".");
  if (error.keyword === "required") return "missing_field";
  if (error.keyword === "type") return "invalid_type";
  if (error.keyword === "pattern" && path.endsWith("id")) return "unstable_id";
  if (
    path.includes("Cardinality") ||
    path.endsWith("variadic") ||
    path.endsWith("repeatable")
  ) {
    return "invalid_cardinality";
  }
  return "invalid_value";
}

function schemaIssueDetail(
  error: ErrorObject,
  messages: CliManifestMessages,
): string {
  if (error.keyword === "type" && typeof error.params.type === "string") {
    return messages.schemaTypeConstraint(error.params.type);
  }
  return messages.schemaConstraint(error.keyword);
}

function schemaDiagnostics(
  errors: readonly ErrorObject[],
  messages: CliManifestMessages,
): CliManifestDiagnostic[] {
  return errors
    .filter((error) => error.keyword !== "if")
    .map((error) => {
      const path = schemaIssuePath(error);
      return {
        code: schemaIssueCode(error),
        path,
        message:
          error.keyword === "required"
            ? messages.requiredField(String(path.at(-1)))
            : messages.contractFailure(
                path.join(".") || "manifest",
                schemaIssueDetail(error, messages),
              ),
      };
    });
}

function addDiagnostic(
  diagnostics: CliManifestDiagnostic[],
  code: CliManifestDiagnosticCode,
  path: Path,
  message: string,
): void {
  diagnostics.push({ code, path, message });
}

function registerID(
  id: string,
  path: Path,
  seenIDs: Map<string, Path>,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  const firstPath = seenIDs.get(id);
  if (firstPath) {
    addDiagnostic(
      diagnostics,
      "duplicate_id",
      path,
      messages.duplicateId(id, firstPath.join(".")),
    );
    return;
  }
  seenIDs.set(id, path);
}

function validateRecordIdentities(
  manifest: CliManifest,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  const seenIDs = new Map<string, Path>();
  for (const [commandKey, command] of Object.entries(manifest.commands)) {
    const commandPath = ["commands", commandKey, "id"] as const;
    registerID(command.id, commandPath, seenIDs, diagnostics, messages);
    if (commandKey !== command.id) {
      addDiagnostic(
        diagnostics,
        "unstable_id",
        commandPath,
        messages.commandKeyMismatch(commandKey, command.id),
      );
    }
    for (const [collection, records] of [
      ["arguments", command.arguments],
      ["flags", command.flags],
    ] as const) {
      for (const [inputKey, input] of Object.entries(records ?? {})) {
        const inputPath = [
          "commands",
          commandKey,
          collection,
          inputKey,
          "id",
        ] as const;
        registerID(input.id, inputPath, seenIDs, diagnostics, messages);
        if (inputKey !== input.id) {
          addDiagnostic(
            diagnostics,
            "unstable_id",
            inputPath,
            messages.inputKeyMismatch(inputKey, input.id),
          );
        }
      }
    }
  }
}

function validateArgumentCardinality(
  command: CliCommand,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  const ordered = Object.values(command.arguments ?? {}).sort(
    (left, right) => left.position - right.position,
  );
  for (const [index, argument] of ordered.entries()) {
    const path = ["commands", command.id, "arguments", argument.id] as const;
    if (argument.position !== index) {
      addDiagnostic(
        diagnostics,
        "invalid_cardinality",
        [...path, "position"],
        messages.argumentPosition(index),
      );
    }
    if (argument.variadic && index !== ordered.length - 1) {
      addDiagnostic(
        diagnostics,
        "invalid_cardinality",
        [...path, "variadic"],
        messages.finalArgumentVariadic(),
      );
    }
    const impossible =
      argument.minCardinality < 0 ||
      (argument.maxCardinality !== -1 &&
        argument.maxCardinality < argument.minCardinality) ||
      (argument.required && argument.minCardinality < 1) ||
      (!argument.required && argument.minCardinality > 0) ||
      (argument.variadic && argument.maxCardinality !== -1) ||
      (!argument.variadic && argument.maxCardinality === -1);
    if (impossible) {
      addDiagnostic(
        diagnostics,
        "invalid_cardinality",
        path,
        messages.argumentCardinality(argument.id),
      );
    }
  }
}

function validateRelationships(
  command: CliCommand,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  const inputKinds = new Map<string, "argument" | "flag">([
    ...Object.keys(command.arguments ?? {}).map(
      (id) => [id, "argument"] as const,
    ),
    ...Object.keys(command.flags ?? {}).map((id) => [id, "flag"] as const),
  ]);
  for (const relationship of Object.values(command.relationships ?? {})) {
    const path = [
      "commands",
      command.id,
      "relationships",
      relationship.id,
    ] as const;
    const references = [
      ...relationship.participants.map((participant, index) => ({
        path: [...path, "participants", index] as const,
        reference: participant,
      })),
      ...(relationship.when
        ? [{ path: [...path, "when"] as const, reference: relationship.when }]
        : []),
    ];
    for (const { path: referencePath, reference } of references) {
      if (inputKinds.get(reference.id) !== reference.type) {
        addDiagnostic(
          diagnostics,
          "invalid_reference",
          [...referencePath, "id"],
          messages.relationshipParticipant(reference.id, reference.type),
        );
      }
    }
  }
}

function flagSignature(flag: CliFlag): string {
  return JSON.stringify({
    acceptedSources: flag.acceptedSources,
    aliases: flag.aliases,
    binding: flag.binding,
    changedDefault: flag.changedDefault,
    completion: flag.completion,
    default: flag.default,
    defaultValue: flag.defaultValue,
    enum: flag.enum,
    long: flag.long,
    maxCardinality: flag.maxCardinality,
    minCardinality: flag.minCardinality,
    noOptionDefault: flag.noOptionDefault,
    noOptionDefaultValue: flag.noOptionDefaultValue,
    normalization: flag.normalization,
    repeatable: flag.repeatable,
    required: flag.required,
    shorthand: flag.shorthand,
    handlerBindingId: flag.handlerBindingId,
    valueType: flag.valueType,
    visibility: flag.visibility,
  });
}

function inheritedFlagSource(
  command: CliCommand,
  inheritedFlag: CliFlag,
  commandsByPath: ReadonlyMap<string, CliCommand>,
): CliFlag | undefined {
  const segments = command.path.split(" ");
  for (let length = segments.length - 1; length > 0; length -= 1) {
    const ancestor = commandsByPath.get(segments.slice(0, length).join(" "));
    const source = Object.values(ancestor?.flags ?? {}).find(
      (candidate) =>
        candidate.scope === "persistent" &&
        candidate.long === inheritedFlag.long,
    );
    if (source) return source;
  }
  return undefined;
}

function validateHierarchyAndInheritance(
  manifest: CliManifest,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  const commandsByPath = new Map<string, CliCommand>();
  for (const command of Object.values(manifest.commands)) {
    if (commandsByPath.has(command.path)) {
      addDiagnostic(
        diagnostics,
        "invalid_hierarchy",
        ["commands", command.id, "path"],
        messages.duplicateCommandPath(command.path),
      );
    }
    commandsByPath.set(command.path, command);
    const segments = command.path.split(" ");
    if (
      segments.some((segment) => segment.length === 0) ||
      segments.at(-1) !== command.name
    ) {
      addDiagnostic(
        diagnostics,
        "invalid_hierarchy",
        ["commands", command.id, "path"],
        messages.commandPathName(command.path, command.name),
      );
    }
  }
  if (!commandsByPath.has(manifest.rootPath)) {
    addDiagnostic(
      diagnostics,
      "invalid_hierarchy",
      ["rootPath"],
      messages.missingRootCommand(manifest.rootPath),
    );
  }
  for (const command of Object.values(manifest.commands)) {
    if (command.path !== manifest.rootPath) {
      const parentPath = command.path.split(" ").slice(0, -1).join(" ");
      if (!commandsByPath.has(parentPath)) {
        addDiagnostic(
          diagnostics,
          "invalid_hierarchy",
          ["commands", command.id, "path"],
          messages.missingParentCommand(parentPath),
        );
      }
      if (!command.path.startsWith(`${manifest.rootPath} `)) {
        addDiagnostic(
          diagnostics,
          "invalid_hierarchy",
          ["commands", command.id, "path"],
          messages.commandOutsideRoot(command.path, manifest.rootPath),
        );
      }
    }
    for (const flag of Object.values(command.flags ?? {})) {
      if (flag.scope !== "inherited") continue;
      const source = inheritedFlagSource(command, flag, commandsByPath);
      if (!source || flagSignature(source) !== flagSignature(flag)) {
        addDiagnostic(
          diagnostics,
          "contradictory_inheritance",
          ["commands", command.id, "flags", flag.id, "scope"],
          source
            ? messages.inheritedFlagContradiction(flag.long)
            : messages.inheritedFlagMissing(flag.long),
        );
      }
    }
    validateArgumentCardinality(command, diagnostics, messages);
    diagnostics.push(...validateCliInputSemantics(command, messages));
    validateRelationships(command, diagnostics, messages);
  }
}

function validateRootPath(
  rootPath: string,
  diagnostics: CliManifestDiagnostic[],
  messages: CliManifestMessages,
): void {
  if (rootPath.split(" ").some((segment) => segment.length === 0)) {
    addDiagnostic(
      diagnostics,
      "invalid_hierarchy",
      ["rootPath"],
      messages.invalidRootSpacing(),
    );
  }
}

function sortedDiagnostics(
  diagnostics: CliManifestDiagnostic[],
): CliManifestDiagnostic[] {
  return diagnostics.sort((left, right) => {
    const pathOrder = left.path.join(".").localeCompare(right.path.join("."));
    return pathOrder === 0 ? left.code.localeCompare(right.code) : pathOrder;
  });
}

export function loadingCliManifest(): CliManifestLoadState {
  return { status: "loading" };
}

export function loadCliManifest(
  input: unknown,
  locale?: string | null,
): CliManifestLoadState {
  const messages = getCliManifestMessages(locale);
  if (
    isRecord(input) &&
    typeof input.formatVersion === "string" &&
    input.formatVersion !== SUPPORTED_CLI_MANIFEST_FORMAT_VERSION
  ) {
    return {
      status: "unsupported-version",
      receivedVersion: input.formatVersion,
      supportedVersions: [SUPPORTED_CLI_MANIFEST_FORMAT_VERSION],
    };
  }
  if (!validateManifestShape(input)) {
    return {
      status: "invalid-contract",
      diagnostics: sortedDiagnostics(
        schemaDiagnostics(validateManifestShape.errors ?? [], messages),
      ),
    };
  }

  const manifest = input as CliManifest;
  const diagnostics: CliManifestDiagnostic[] = [];
  validateRootPath(manifest.rootPath, diagnostics, messages);
  if (Object.keys(manifest.commands).length === 0) {
    return diagnostics.length > 0
      ? {
          status: "invalid-contract",
          diagnostics: sortedDiagnostics(diagnostics),
        }
      : { status: "empty", manifest };
  }
  validateRecordIdentities(manifest, diagnostics, messages);
  validateHierarchyAndInheritance(manifest, diagnostics, messages);
  return diagnostics.length > 0
    ? {
        status: "invalid-contract",
        diagnostics: sortedDiagnostics(diagnostics),
      }
    : { status: "ready", manifest };
}

export function loadPublishedCliManifest(
  locale?: string | null,
): CliManifestLoadState {
  return loadCliManifest(publishedCliManifestArtifact, locale);
}
