import type { CliManifestMessages } from "../messages/cli-manifest";
import type {
  CliCommand,
  CliFlag,
  CliInputValue,
  CliInputValueType,
  CliManifestDiagnostic,
} from "./cli-manifest-types";

export type CliInheritedFlagSource = {
  readonly command: CliCommand;
  readonly flag: CliFlag;
};

/** Resolves the exact persistent ancestor named by the canonical inheritance reference. */
export function resolveInheritedFlagSource(
  command: CliCommand,
  inheritedFlag: CliFlag,
  commands: readonly CliCommand[],
): CliInheritedFlagSource | undefined {
  if (
    inheritedFlag.scope !== "inherited" ||
    !inheritedFlag.inheritedFromInputId
  ) {
    return undefined;
  }

  for (const candidateCommand of commands) {
    if (!command.path.startsWith(`${candidateCommand.path} `)) continue;
    const candidateFlag: CliFlag | undefined =
      candidateCommand.flags?.[inheritedFlag.inheritedFromInputId];
    if (
      candidateFlag?.id === inheritedFlag.inheritedFromInputId &&
      candidateFlag.scope === "persistent"
    ) {
      return { command: candidateCommand, flag: candidateFlag };
    }
  }
  return undefined;
}

function typedValueMatches(
  value: CliInputValue,
  valueType: CliInputValueType,
): boolean {
  const expectedMember = valueType === "bool" ? "boolean" : valueType;
  return Object.hasOwn(value, expectedMember);
}

export function validateCliInputSemantics(
  command: CliCommand,
  messages: CliManifestMessages,
): CliManifestDiagnostic[] {
  const diagnostics: CliManifestDiagnostic[] = [];
  for (const argument of Object.values(command.arguments ?? {})) {
    if (
      argument.defaultValue &&
      !typedValueMatches(argument.defaultValue, argument.valueType)
    ) {
      diagnostics.push({
        code: "invalid_value",
        path: [
          "commands",
          command.id,
          "arguments",
          argument.id,
          "defaultValue",
        ],
        message: messages.inputDefaultType(argument.id, argument.valueType),
      });
    }
  }
  for (const flag of Object.values(command.flags ?? {})) {
    if (flag.kind !== "named") continue;
    if (
      (flag.maxCardinality !== -1 &&
        flag.maxCardinality < flag.minCardinality) ||
      (flag.required && flag.minCardinality < 1) ||
      (!flag.required && flag.minCardinality > 0) ||
      flag.repeatable !==
        (flag.maxCardinality === -1 || flag.maxCardinality > 1)
    ) {
      diagnostics.push({
        code: "invalid_cardinality",
        path: ["commands", command.id, "flags", flag.id],
        message: messages.flagCardinality(flag.id),
      });
    }
    for (const [field, value] of [
      ["defaultValue", flag.defaultValue],
      ["noOptionDefaultValue", flag.noOptionDefaultValue],
    ] as const) {
      if (value && !typedValueMatches(value, flag.valueType)) {
        diagnostics.push({
          code: "invalid_value",
          path: ["commands", command.id, "flags", flag.id, field],
          message: messages.inputDefaultType(flag.id, flag.valueType),
        });
      }
    }
  }
  return diagnostics;
}
