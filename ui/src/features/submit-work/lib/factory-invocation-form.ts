import type { components } from "../../../api/generated/openapi";

type FactoryInvocationOutputContract =
  components["schemas"]["FactoryInvocationOutputContract"];
type FactoryInvocationParameter = components["schemas"]["FactoryInvocationParameter"];
type FactoryInvocationSignature =
  components["schemas"]["FactoryInvocationSignature"];

export interface InvocationFieldModel {
  aliases: string[];
  choices: string[];
  defaultValues: string[];
  description?: string;
  externalName?: string;
  hasNamedBinding: boolean;
  hasStdinBinding: boolean;
  kind: "boolean" | "choice" | "repeated" | "text";
  label: string;
  name: string;
  pathHint?: "directory" | "file" | "path";
  position?: number;
  required: boolean;
}

export interface InvocationFormProjection {
  examples: components["schemas"]["FactoryInvocationExample"][];
  fields: InvocationFieldModel[];
  outputContract?: FactoryInvocationOutputContract;
}

export interface InvocationFormMessagesLike {
  repeatedItemRequired: string;
  requiredFieldMessage: (label: string) => string;
}

export function projectInvocationForm(
  signature: FactoryInvocationSignature | undefined,
  examples: components["schemas"]["FactoryInvocationExample"][] = [],
): InvocationFormProjection {
  const parameters = signature?.parameters ?? [];
  return {
    examples,
    fields: parameters.map(projectInvocationField),
    outputContract: signature?.outputContract,
  };
}

export function serializeInvocationArgs(
  fields: readonly InvocationFieldModel[],
  values: Record<string, string[] | undefined>,
): Record<string, string | string[]> {
  const args: Record<string, string | string[]> = {};

  for (const field of fields) {
    const normalizedValues = normalizeDraftValues(values[field.name]);
    if (normalizedValues.length === 0) {
      continue;
    }

    args[field.name] =
      field.kind === "repeated" ? normalizedValues : normalizedValues[0];
  }

  return args;
}

export function collectInvocationFieldErrors(
  fields: readonly InvocationFieldModel[],
  values: Record<string, string[] | undefined>,
  messages: InvocationFormMessagesLike,
): Record<string, string> {
  const fieldErrors: Record<string, string> = {};

  for (const field of fields) {
    const normalizedValues = normalizeDraftValues(values[field.name]);
    if (
      field.required &&
      normalizedValues.length === 0 &&
      field.defaultValues.length === 0
    ) {
      fieldErrors[field.name] = messages.requiredFieldMessage(field.label);
      continue;
    }

    if (
      field.kind === "repeated" &&
      hasBlankRepeatedRow(values[field.name]) &&
      normalizedValues.length > 0
    ) {
      fieldErrors[field.name] = messages.repeatedItemRequired;
    }
  }

  return fieldErrors;
}

export function extractInvocationFieldError(
  fields: readonly InvocationFieldModel[],
  message: string,
): { fieldName: string; message: string } | null {
  const quotedParameter = message.match(/parameter "([^"]+)"/i)?.[1]?.trim();
  if (quotedParameter) {
    const field = fields.find((entry) => entry.name === quotedParameter);
    if (field) {
      return {
        fieldName: field.name,
        message,
      };
    }
  }

  return null;
}

function projectInvocationField(
  parameter: FactoryInvocationParameter,
): InvocationFieldModel {
  const aliases = parameter.aliases ?? [];
  const choices = parameter.choices ?? [];
  const defaultValues =
    parameter.defaultValues && parameter.defaultValues.length > 0
      ? parameter.defaultValues
      : parameter.defaultValue
        ? [parameter.defaultValue]
        : [];
  const position = firstPositionalBinding(parameter);

  return {
    aliases,
    choices,
    defaultValues,
    description: parameter.description,
    externalName: parameter.externalName,
    hasNamedBinding: hasBinding(parameter, "NAMED") || hasBinding(parameter, "NAMED_REST"),
    hasStdinBinding: hasBinding(parameter, "STDIN"),
    kind: resolveInvocationFieldKind(parameter),
    label: parameter.externalName?.trim() || parameter.name,
    name: parameter.name,
    pathHint: resolvePathHint(parameter.typeHint),
    position,
    required: parameter.required ?? false,
  };
}

function resolveInvocationFieldKind(
  parameter: FactoryInvocationParameter,
): InvocationFieldModel["kind"] {
  if (parameter.valueMode === "REPEATED" || parameter.valueMode === "VARIADIC") {
    return "repeated";
  }
  if (parameter.choices && parameter.choices.length > 0) {
    return "choice";
  }
  if (parameter.typeHint === "BOOLEAN_STRING") {
    return "boolean";
  }
  return "text";
}

function resolvePathHint(
  typeHint: FactoryInvocationParameter["typeHint"],
): InvocationFieldModel["pathHint"] | undefined {
  switch (typeHint) {
    case "DIRECTORY_PATH":
      return "directory";
    case "FILE_PATH":
      return "file";
    case "PATH":
      return "path";
    default:
      return undefined;
  }
}

function hasBinding(
  parameter: FactoryInvocationParameter,
  kind: components["schemas"]["FactoryInvocationParameterBindingKind"],
): boolean {
  return parameter.bindings?.some((binding) => binding.kind === kind) ?? false;
}

function firstPositionalBinding(
  parameter: FactoryInvocationParameter,
): number | undefined {
  return parameter.bindings?.find((binding) => binding.kind === "POSITIONAL")
    ?.position;
}

function normalizeDraftValues(values: string[] | undefined): string[] {
  return (values ?? []).filter((value) => value.trim().length > 0);
}

function hasBlankRepeatedRow(values: string[] | undefined): boolean {
  return (values ?? []).some((value) => value.trim().length === 0);
}
