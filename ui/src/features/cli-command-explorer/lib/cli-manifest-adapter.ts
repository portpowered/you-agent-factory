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
import { getCliManifestMessages } from "../messages/cli-manifest";
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

function schemaDiagnostics(
  errors: readonly ErrorObject[],
): CliManifestDiagnostic[] {
  const messages = getCliManifestMessages();
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
                error.message ?? error.keyword,
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
): void {
  const firstPath = seenIDs.get(id);
  if (firstPath) {
    addDiagnostic(
      diagnostics,
      "duplicate_id",
      path,
      `Stable id ${id} duplicates ${firstPath.join(".")}.`,
    );
    return;
  }
  seenIDs.set(id, path);
}

function validateRecordIdentities(
  manifest: CliManifest,
  diagnostics: CliManifestDiagnostic[],
): void {
  const seenIDs = new Map<string, Path>();
  for (const [commandKey, command] of Object.entries(manifest.commands)) {
    const commandPath = ["commands", commandKey, "id"] as const;
    registerID(command.id, commandPath, seenIDs, diagnostics);
    if (commandKey !== command.id) {
      addDiagnostic(
        diagnostics,
        "unstable_id",
        commandPath,
        `Command key ${commandKey} must match stable id ${command.id}.`,
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
        registerID(input.id, inputPath, seenIDs, diagnostics);
        if (inputKey !== input.id) {
          addDiagnostic(
            diagnostics,
            "unstable_id",
            inputPath,
            `Input key ${inputKey} must match stable id ${input.id}.`,
          );
        }
      }
    }
  }
}

function validateArgumentCardinality(
  command: CliCommand,
  diagnostics: CliManifestDiagnostic[],
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
        `Argument positions must be unique and contiguous from zero; expected ${index}.`,
      );
    }
    if (argument.variadic && index !== ordered.length - 1) {
      addDiagnostic(
        diagnostics,
        "invalid_cardinality",
        [...path, "variadic"],
        "Only the final positional argument may be variadic.",
      );
    }
    const impossible =
      argument.minCardinality < 0 ||
      (argument.maxCardinality !== -1 &&
        argument.maxCardinality < argument.minCardinality) ||
      (argument.required && argument.minCardinality < 1) ||
      (argument.variadic && argument.maxCardinality !== -1) ||
      (!argument.variadic && argument.maxCardinality === -1);
    if (impossible) {
      addDiagnostic(
        diagnostics,
        "invalid_cardinality",
        path,
        `Argument ${argument.id} has contradictory required, minimum, maximum, or variadic cardinality.`,
      );
    }
  }
}

function validateRelationships(
  command: CliCommand,
  diagnostics: CliManifestDiagnostic[],
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
          `Relationship participant ${reference.id} does not resolve to a ${reference.type} on this command.`,
        );
      }
    }
  }
}

function flagSignature(flag: CliFlag): string {
  return JSON.stringify({
    aliases: flag.aliases,
    binding: flag.binding,
    changedDefault: flag.changedDefault,
    completion: flag.completion,
    default: flag.default,
    enum: flag.enum,
    long: flag.long,
    noOptionDefault: flag.noOptionDefault,
    normalization: flag.normalization,
    repeatable: flag.repeatable,
    required: flag.required,
    shorthand: flag.shorthand,
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
): void {
  const commandsByPath = new Map<string, CliCommand>();
  for (const command of Object.values(manifest.commands)) {
    if (commandsByPath.has(command.path)) {
      addDiagnostic(
        diagnostics,
        "invalid_hierarchy",
        ["commands", command.id, "path"],
        `Command path ${command.path} is duplicated.`,
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
        `Command path ${command.path} must contain non-empty segments and end with command name ${command.name}.`,
      );
    }
  }
  if (!commandsByPath.has(manifest.rootPath)) {
    addDiagnostic(
      diagnostics,
      "invalid_hierarchy",
      ["rootPath"],
      `Root path ${manifest.rootPath} does not resolve to a command.`,
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
          `Parent command path ${parentPath || "<empty>"} does not resolve.`,
        );
      }
      if (!command.path.startsWith(`${manifest.rootPath} `)) {
        addDiagnostic(
          diagnostics,
          "invalid_hierarchy",
          ["commands", command.id, "path"],
          `Command path ${command.path} is outside root ${manifest.rootPath}.`,
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
            ? `Inherited flag --${flag.long} contradicts its persistent ancestor definition.`
            : `Inherited flag --${flag.long} has no persistent ancestor definition.`,
        );
      }
    }
    validateArgumentCardinality(command, diagnostics);
    validateRelationships(command, diagnostics);
  }
}

function validateRootPath(
  rootPath: string,
  diagnostics: CliManifestDiagnostic[],
): void {
  if (rootPath.split(" ").some((segment) => segment.length === 0)) {
    addDiagnostic(
      diagnostics,
      "invalid_hierarchy",
      ["rootPath"],
      "Root path must contain non-empty command segments separated by single spaces.",
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

export function loadCliManifest(input: unknown): CliManifestLoadState {
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
        schemaDiagnostics(validateManifestShape.errors ?? []),
      ),
    };
  }

  const manifest = input as CliManifest;
  const diagnostics: CliManifestDiagnostic[] = [];
  validateRootPath(manifest.rootPath, diagnostics);
  if (Object.keys(manifest.commands).length === 0) {
    return diagnostics.length > 0
      ? {
          status: "invalid-contract",
          diagnostics: sortedDiagnostics(diagnostics),
        }
      : { status: "empty", manifest };
  }
  validateRecordIdentities(manifest, diagnostics);
  validateHierarchyAndInheritance(manifest, diagnostics);
  return diagnostics.length > 0
    ? {
        status: "invalid-contract",
        diagnostics: sortedDiagnostics(diagnostics),
      }
    : { status: "ready", manifest };
}

export function loadPublishedCliManifest(): CliManifestLoadState {
  return loadCliManifest(publishedCliManifestArtifact);
}
