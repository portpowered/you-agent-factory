import type {
  CliCommandInputProjection,
  CliCommandProjection,
  CliRelationshipProjection,
} from "./cli-command-projection";
import type {
  CliArgument,
  CliFlag,
  CliInputValue,
  CliInputValueType,
} from "./cli-manifest-types";

export type CliControlValue = boolean | string | readonly string[];
export type CliControlValues = Readonly<Record<string, CliControlValue>>;

type CliControlBase = {
  readonly id: string;
  readonly inputId: string;
  readonly label: string;
  readonly required: boolean;
  readonly inherited: boolean;
  readonly defaultValue: CliControlValue;
  readonly cardinality: CliCommandInputProjection["cardinality"];
  readonly relationshipIds: readonly string[];
};

export type CliStaticControl = CliControlBase &
  (
    | { readonly kind: "boolean"; readonly defaultValue: boolean }
    | {
        readonly kind: "choice";
        readonly defaultValue: string;
        readonly choices: readonly string[];
      }
    | {
        readonly kind: "number" | "text";
        readonly defaultValue: string;
      }
    | {
        readonly kind: "repeated";
        readonly defaultValue: readonly string[];
        readonly valueKind: "choice" | "number" | "text";
        readonly choices?: readonly string[];
      }
  );

export type CliControlModel = {
  readonly commandId: string;
  readonly controls: readonly CliStaticControl[];
  readonly relationships: readonly CliRelationshipProjection[];
};

export type CliControlProjectionResult =
  | { readonly status: "ready"; readonly model: CliControlModel }
  | {
      readonly status: "unsupported-input";
      readonly inputId: string;
      readonly valueType: string;
    };

export type CliControlViolation = {
  readonly code: "cardinality" | "relationship";
  readonly inputId: string;
  readonly relationshipId?: string;
  readonly relationshipKind?: CliRelationshipProjection["kind"];
  readonly relatedInputIds: readonly string[];
};

function inputLabel(input: CliCommandInputProjection): string {
  return input.kind === "argument"
    ? `<${(input.manifestInput as CliArgument).name}>`
    : `--${(input.manifestInput as CliFlag).long}`;
}

function relationshipIds(
  inputId: string,
  relationships: readonly CliRelationshipProjection[],
): string[] {
  return relationships
    .filter(
      (relationship) =>
        relationship.participants.some(
          (participant) => participant.inputId === inputId,
        ) || relationship.when?.inputId === inputId,
    )
    .map((relationship) => relationship.id);
}

function typedDefault(
  value: CliInputValue,
): boolean | string | readonly string[] {
  if ("boolean" in value) return value.boolean;
  if ("int" in value) return String(value.int);
  if ("int64" in value) return String(value.int64);
  if ("stringArray" in value) return value.stringArray;
  return value.string;
}

function inputDefault(input: CliArgument | CliFlag): CliControlValue {
  if (input.defaultValue) return typedDefault(input.defaultValue);
  if ("default" in input) {
    if (input.valueType === "bool") return input.default === "true";
    if (input.valueType === "stringArray") {
      return input.default ? [input.default] : [];
    }
    return input.default ?? "";
  }
  if (input.valueType === "bool") return false;
  if (input.valueType === "stringArray") return [];
  return "";
}

function repeatedDefault(value: CliControlValue): readonly string[] {
  if (typeof value === "boolean") return value ? ["true"] : [];
  if (typeof value === "string") return value ? [value] : [];
  return value;
}

function repeatedValueKind(
  valueType: CliInputValueType,
  choices: readonly string[] | undefined,
): "choice" | "number" | "text" {
  if (choices && choices.length > 0) return "choice";
  return valueType === "int" || valueType === "int64" ? "number" : "text";
}

function choiceDefault(value: CliControlValue): string {
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "string") return value;
  return value[0] ?? "";
}

function projectControl(
  input: CliCommandInputProjection,
  relationships: readonly CliRelationshipProjection[],
): CliStaticControl | CliControlProjectionResult {
  const manifestInput = input.manifestInput;
  const defaultValue = inputDefault(manifestInput);
  const base = {
    id: `cli-control-${input.id.replaceAll(".", "-")}`,
    inputId: input.id,
    label: inputLabel(input),
    required: manifestInput.required,
    inherited: input.inherited,
    cardinality: input.cardinality,
    relationshipIds: relationshipIds(input.id, relationships),
  } as const;

  const repeated =
    manifestInput.valueType === "stringArray" ||
    input.cardinality.maximum === null ||
    input.cardinality.maximum > 1;
  if (
    !["bool", "int", "int64", "string", "stringArray"].includes(
      manifestInput.valueType,
    )
  ) {
    return {
      status: "unsupported-input",
      inputId: input.id,
      valueType: manifestInput.valueType,
    };
  }
  if (repeated) {
    const choices =
      manifestInput.enum ??
      (manifestInput.valueType === "bool" ? ["true", "false"] : undefined);
    return {
      ...base,
      kind: "repeated",
      valueKind: repeatedValueKind(manifestInput.valueType, choices),
      ...(choices ? { choices } : {}),
      defaultValue: repeatedDefault(defaultValue),
    };
  }
  if (manifestInput.enum && manifestInput.enum.length > 0) {
    return {
      ...base,
      kind: "choice",
      choices: manifestInput.enum,
      defaultValue: choiceDefault(defaultValue),
    };
  }
  const valueType: string = manifestInput.valueType;
  switch (valueType) {
    case "bool":
      return {
        ...base,
        kind: "boolean",
        defaultValue: defaultValue === true,
      };
    case "int":
    case "int64":
      return {
        ...base,
        kind: "number",
        defaultValue: typeof defaultValue === "string" ? defaultValue : "",
      };
    case "string":
      return {
        ...base,
        kind: "text",
        defaultValue: typeof defaultValue === "string" ? defaultValue : "",
      };
    case "stringArray":
      throw new Error("String-array inputs must project as repeated controls.");
    default:
      return {
        status: "unsupported-input",
        inputId: input.id,
        valueType,
      };
  }
}

export function projectCliCommandControls(
  command: CliCommandProjection,
): CliControlProjectionResult {
  const visibleInputs = command.effectiveInputs.filter(
    (input) =>
      !("visibility" in input.manifestInput) ||
      input.manifestInput.visibility !== "hidden",
  );
  const visibleInputIds = new Set(visibleInputs.map(({ id }) => id));
  const visibleRelationships = command.relationships.filter(
    (relationship) =>
      relationship.participants.every(({ inputId }) =>
        visibleInputIds.has(inputId),
      ) &&
      (!relationship.when || visibleInputIds.has(relationship.when.inputId)),
  );
  const controls: CliStaticControl[] = [];
  for (const input of visibleInputs) {
    const projected = projectControl(input, visibleRelationships);
    if ("status" in projected) return projected;
    controls.push(projected);
  }
  return {
    status: "ready",
    model: {
      commandId: command.id,
      controls,
      relationships: visibleRelationships,
    },
  };
}

function valueCount(value: CliControlValue | undefined): number {
  if (typeof value === "boolean") return value ? 1 : 0;
  if (typeof value === "string") return value.trim() ? 1 : 0;
  return value?.filter((entry) => entry.trim()).length ?? 0;
}

function violatesRelationship(
  relationship: CliRelationshipProjection,
  active: ReadonlySet<string>,
): boolean {
  const participantIds = relationship.participants.map(
    ({ inputId }) => inputId,
  );
  const activeCount = participantIds.filter((id) => active.has(id)).length;
  switch (relationship.kind) {
    case "at-least-one":
      return activeCount === 0;
    case "conflict":
    case "mutually-exclusive":
      return activeCount > 1;
    case "required-together":
      return activeCount > 0 && activeCount < participantIds.length;
    case "conditional":
    case "dependency":
      return (
        Boolean(relationship.when && active.has(relationship.when.inputId)) &&
        activeCount < participantIds.length
      );
  }
}

export function validateCliControlValues(
  model: CliControlModel,
  values: CliControlValues,
  explicitlySetInputIds: ReadonlySet<string> = new Set(Object.keys(values)),
): readonly CliControlViolation[] {
  const violations: CliControlViolation[] = [];
  const active = new Set<string>();
  for (const control of model.controls) {
    const count = valueCount(values[control.inputId]);
    if (count > 0 && explicitlySetInputIds.has(control.inputId)) {
      active.add(control.inputId);
    }
    if (
      count < control.cardinality.minimum ||
      (control.cardinality.maximum !== null &&
        count > control.cardinality.maximum)
    ) {
      violations.push({
        code: "cardinality",
        inputId: control.inputId,
        relatedInputIds: [],
      });
    }
  }
  for (const relationship of model.relationships) {
    if (!violatesRelationship(relationship, active)) continue;
    const participantIds = relationship.participants.map(
      ({ inputId }) => inputId,
    );
    for (const inputId of participantIds) {
      const relatedInputIds = participantIds.filter((id) => id !== inputId);
      if (
        relationship.when &&
        !relatedInputIds.includes(relationship.when.inputId)
      ) {
        relatedInputIds.push(relationship.when.inputId);
      }
      violations.push({
        code: "relationship",
        inputId,
        relationshipId: relationship.id,
        relationshipKind: relationship.kind,
        relatedInputIds,
      });
    }
  }
  return violations;
}
