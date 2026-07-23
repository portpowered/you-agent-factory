import { useMemo, useState } from "react";

import { Button } from "../../../components/ui/button";
import { Checkbox } from "../../../components/ui/checkbox";
import { OptionalEnumSelect } from "../../../components/ui/enum-select";
import {
  FormDescription,
  FormError,
  FormField,
  FormLabel,
} from "../../../components/ui/form-field";
import { Input } from "../../../components/ui/input";
import {
  type CliControlModel,
  type CliControlValue,
  type CliStaticControl,
  validateCliControlValues,
} from "../lib/cli-control-projection";
import { getCliControlMessages } from "../messages/cli-controls";

export interface StaticCliControlsProps {
  readonly locale?: string | null;
  readonly model: CliControlModel;
}

function initialValues(
  model: CliControlModel,
): Record<string, CliControlValue> {
  return Object.fromEntries(
    model.controls.map((control) => [control.inputId, control.defaultValue]),
  );
}

export function StaticCliControls({ locale, model }: StaticCliControlsProps) {
  const messages = getCliControlMessages(locale);
  const [values, setValues] = useState<Record<string, CliControlValue>>(() =>
    initialValues(model),
  );
  const [explicitlySetInputIds, setExplicitlySetInputIds] = useState<
    ReadonlySet<string>
  >(() => new Set());
  const violations = useMemo(
    () => validateCliControlValues(model, values, explicitlySetInputIds),
    [explicitlySetInputIds, model, values],
  );
  const labels = new Map(
    model.controls.map((control) => [control.inputId, control.label] as const),
  );
  const updateValue = (inputId: string, value: CliControlValue) => {
    setValues((current) => ({ ...current, [inputId]: value }));
    setExplicitlySetInputIds((current) => new Set(current).add(inputId));
  };

  return (
    <div className="grid gap-4">
      {model.controls.map((control) => {
        const controlViolations = violations.filter(
          (violation) => violation.inputId === control.inputId,
        );
        const descriptionId = `${control.id}-description`;
        const errorId = `${control.id}-error`;
        const describedBy = [
          descriptionId,
          controlViolations.length > 0 ? errorId : undefined,
        ]
          .filter(Boolean)
          .join(" ");
        const relatedLabels = controlViolations
          .flatMap((violation) => violation.relatedInputIds)
          .map((inputId) => labels.get(inputId) ?? inputId)
          .join(", ");
        const guidanceLabels = model.relationships
          .filter((relationship) =>
            control.relationshipIds.includes(relationship.id),
          )
          .flatMap((relationship) =>
            [
              ...relationship.participants,
              ...(relationship.when ? [relationship.when] : []),
            ]
              .filter(({ inputId }) => inputId !== control.inputId)
              .map(({ inputId }) => labels.get(inputId) ?? inputId),
          )
          .filter((label, index, all) => all.indexOf(label) === index)
          .join(", ");
        const description = [
          control.required ? messages.required : undefined,
          control.inherited ? messages.inherited : undefined,
          typeof control.defaultValue === "string" && control.defaultValue
            ? messages.defaultValue(control.defaultValue)
            : undefined,
          guidanceLabels
            ? messages.relationshipGuidance(guidanceLabels)
            : undefined,
        ]
          .filter(Boolean)
          .join(" · ");
        return (
          <FormField key={control.inputId}>
            <ControlInput
              control={control}
              describedBy={describedBy}
              errorMessageId={
                controlViolations.length > 0 ? errorId : undefined
              }
              invalid={controlViolations.length > 0}
              messages={messages}
              onChange={(value) => updateValue(control.inputId, value)}
              value={values[control.inputId] ?? control.defaultValue}
            />
            <FormDescription id={descriptionId}>{description}</FormDescription>
            {controlViolations.length > 0 ? (
              <FormError id={errorId}>
                {controlViolations.some(({ code }) => code === "cardinality")
                  ? messages.cardinalityError(
                      control.label,
                      control.cardinality.minimum,
                      control.cardinality.maximum,
                    )
                  : messages.relationshipError(
                      controlViolations.find(
                        ({ relationshipKind }) => relationshipKind,
                      )?.relationshipKind ?? "conflict",
                      relatedLabels,
                    )}
              </FormError>
            ) : null}
          </FormField>
        );
      })}
    </div>
  );
}

interface ControlInputProps {
  readonly control: CliStaticControl;
  readonly describedBy: string;
  readonly errorMessageId?: string;
  readonly invalid: boolean;
  readonly messages: ReturnType<typeof getCliControlMessages>;
  readonly onChange: (value: CliControlValue) => void;
  readonly value: CliControlValue;
}

function ControlInput({
  control,
  describedBy,
  errorMessageId,
  invalid,
  messages,
  onChange,
  value,
}: ControlInputProps) {
  if (control.kind === "boolean") {
    return (
      <div className="flex items-center gap-2">
        <Checkbox
          aria-describedby={describedBy}
          aria-errormessage={errorMessageId}
          aria-invalid={invalid || undefined}
          checked={value === true}
          id={control.id}
          onChange={(event) => onChange(event.currentTarget.checked)}
        />
        <FormLabel htmlFor={control.id}>{control.label}</FormLabel>
      </div>
    );
  }
  if (control.kind === "choice") {
    return (
      <>
        <FormLabel htmlFor={control.id}>{control.label}</FormLabel>
        <OptionalEnumSelect
          aria-describedby={describedBy}
          aria-invalid={invalid || undefined}
          emptyOptionLabel={messages.optionalChoice}
          id={control.id}
          onValueChange={(nextValue) => onChange(nextValue ?? "")}
          options={control.choices.map((choice) => ({
            label: choice,
            value: choice,
          }))}
          value={typeof value === "string" && value ? value : null}
        />
      </>
    );
  }
  if (control.kind === "repeated") {
    const entries = Array.isArray(value) && value.length > 0 ? value : [""];
    return (
      <>
        <FormLabel as="p">{control.label}</FormLabel>
        {entries.map((entry, index) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: controlled CLI values have no durable item identity.
          <div className="flex gap-2" key={`${control.id}-${index}`}>
            {control.valueKind === "choice" && control.choices ? (
              <OptionalEnumSelect
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                aria-label={messages.valuePosition(control.label, index + 1)}
                emptyOptionLabel={messages.optionalChoice}
                id={`${control.id}-${index}`}
                onValueChange={(nextValue) => {
                  const next = [...entries];
                  next[index] = nextValue ?? "";
                  onChange(next);
                }}
                options={control.choices.map((choice) => ({
                  label: choice,
                  value: choice,
                }))}
                value={entry || null}
              />
            ) : (
              <Input
                aria-describedby={describedBy}
                aria-errormessage={errorMessageId}
                aria-invalid={invalid || undefined}
                aria-label={messages.valuePosition(control.label, index + 1)}
                onChange={(event) => {
                  const next = [...entries];
                  next[index] = event.currentTarget.value;
                  onChange(next);
                }}
                type={control.valueKind === "number" ? "number" : "text"}
                value={entry}
              />
            )}
            {entries.length > 1 ? (
              <Button
                aria-label={messages.removeValue(control.label, index + 1)}
                onClick={() =>
                  onChange(entries.filter((_, position) => position !== index))
                }
                type="button"
                tone="ghost"
              >
                −
              </Button>
            ) : null}
          </div>
        ))}
        <Button
          onClick={() => onChange([...entries, ""])}
          type="button"
          tone="secondary"
        >
          {messages.addValue(control.label)}
        </Button>
      </>
    );
  }
  return (
    <>
      <FormLabel htmlFor={control.id}>{control.label}</FormLabel>
      <Input
        aria-describedby={describedBy}
        aria-errormessage={errorMessageId}
        aria-invalid={invalid || undefined}
        id={control.id}
        onChange={(event) => onChange(event.currentTarget.value)}
        required={control.required}
        type={control.kind === "number" ? "number" : "text"}
        value={typeof value === "string" ? value : ""}
      />
    </>
  );
}
