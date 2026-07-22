import type {
  CliCommandInputProjection,
  CliCommandProjection,
  CliRelationshipProjection,
} from "./cli-command-projection";
import type { CliArgument, CliFlag } from "./cli-manifest-types";

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
    | { readonly kind: "repeated"; readonly defaultValue: readonly string[] }
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

function projectControl(
  input: CliCommandInputProjection,
  relationships: readonly CliRelationshipProjection[],
): CliStaticControl | CliControlProjectionResult {
  const manifestInput = input.manifestInput;
  const base = {
    id: `cli-control-${input.id.replaceAll(".", "-")}`,
    inputId: input.id,
    label: inputLabel(input),
    required: manifestInput.required,
    inherited: input.inherited,
    cardinality: input.cardinality,
    relationshipIds: relationshipIds(input.id, relationships),
  } as const;

  if (manifestInput.enum && manifestInput.enum.length > 0) {
    return {
      ...base,
      kind: "choice",
      choices: manifestInput.enum,
      defaultValue: "default" in manifestInput ? manifestInput.default : "",
    };
  }
  if (input.kind === "argument") {
    const argument = manifestInput as CliArgument;
    return argument.variadic
      ? { ...base, kind: "repeated", defaultValue: [] }
      : { ...base, kind: "text", defaultValue: "" };
  }
  const flag = manifestInput as CliFlag;
  const valueType: string = flag.valueType;
  switch (valueType) {
    case "bool":
      return {
        ...base,
        kind: "boolean",
        defaultValue: flag.default === "true",
      };
    case "int":
    case "int64":
      return { ...base, kind: "number", defaultValue: flag.default };
    case "string":
      return { ...base, kind: "text", defaultValue: flag.default };
    case "stringArray":
      return {
        ...base,
        kind: "repeated",
        defaultValue: flag.default ? [flag.default] : [],
      };
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
  const controls: CliStaticControl[] = [];
  for (const input of command.effectiveInputs) {
    const projected = projectControl(input, command.relationships);
    if ("status" in projected) return projected;
    controls.push(projected);
  }
  return {
    status: "ready",
    model: {
      commandId: command.id,
      controls,
      relationships: command.relationships,
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
      return (
        Boolean(relationship.when && active.has(relationship.when.inputId)) &&
        activeCount < participantIds.length
      );
  }
}

export function validateCliControlValues(
  model: CliControlModel,
  values: CliControlValues,
): readonly CliControlViolation[] {
  const violations: CliControlViolation[] = [];
  const active = new Set<string>();
  for (const control of model.controls) {
    const count = valueCount(values[control.inputId]);
    if (count > 0) active.add(control.inputId);
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
      violations.push({
        code: "relationship",
        inputId,
        relationshipId: relationship.id,
        relationshipKind: relationship.kind,
        relatedInputIds: participantIds.filter((id) => id !== inputId),
      });
    }
  }
  return violations;
}
