import {
  EnumSelect,
  OptionalEnumSelect,
} from "@you-agent-factory/components/forms";
import type { ReactNode } from "react";
import {
  FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
} from "../../../../components/prompt-editor";
import {
  FormDescription,
  FormError,
  FormField,
  FormLabel,
} from "../../../../components/ui/form-field";
import { Input } from "../../../../components/ui/input";
import { Textarea } from "../../../../components/ui/textarea";

export function FactoryGraphEditorAddField({
  children,
  error,
  helpText,
  inputId,
  label,
}: {
  children: ReactNode;
  error?: string;
  helpText?: string;
  inputId?: string;
  label: ReactNode;
}) {
  const labelContent = inputId ? (
    <FormLabel htmlFor={inputId}>{label}</FormLabel>
  ) : (
    <FormLabel as="p" className="m-0">
      {label}
    </FormLabel>
  );

  return (
    <FormField className="grid gap-2">
      {labelContent}
      {children}
      {helpText ? (
        <FormDescription id={inputId ? `${inputId}-help` : undefined}>
          {helpText}
        </FormDescription>
      ) : null}
      {error ? (
        <FormError id={inputId ? `${inputId}-error` : undefined}>
          {error}
        </FormError>
      ) : null}
    </FormField>
  );
}

export function FactoryGraphEditorTextareaField({
  error,
  helpText,
  inputId,
  label,
  onChange,
  value,
}: {
  error?: string;
  helpText?: string;
  inputId: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <FactoryGraphEditorAddField
      error={error}
      helpText={helpText}
      inputId={inputId}
      label={label}
    >
      <Textarea
        aria-label={label}
        className={"bg-surface"}
        id={inputId}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      />
    </FactoryGraphEditorAddField>
  );
}

export function FactoryGraphEditorTextField({
  error,
  helpText,
  inputId,
  inputMode,
  label,
  onChange,
  value,
}: {
  error?: string;
  helpText?: string;
  inputId: string;
  inputMode?: "numeric";
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <FactoryGraphEditorAddField
      error={error}
      helpText={helpText}
      inputId={inputId}
      label={label}
    >
      <Input
        aria-label={label}
        className={"bg-surface"}
        id={inputId}
        inputMode={inputMode}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      />
    </FactoryGraphEditorAddField>
  );
}

function buildSelectAriaDescribedBy(
  inputId: string,
  helpText?: string,
  error?: string,
) {
  return (
    [helpText ? `${inputId}-help` : null, error ? `${inputId}-error` : null]
      .filter(Boolean)
      .join(" ") || undefined
  );
}

export function FactoryGraphEditorSelectField({
  error,
  helpText,
  inputId,
  label,
  onChange,
  options,
  value,
}: {
  error?: string;
  helpText?: string;
  inputId: string;
  label: string;
  onChange: (value: string) => void;
  options: Array<{ label: string; value: string }>;
  value: string;
}) {
  const emptyOption = options.find((option) => option.value === "");
  const enumOptions = emptyOption
    ? options.filter((option) => option.value !== "")
    : options;
  const ariaProps = {
    "aria-describedby": buildSelectAriaDescribedBy(inputId, helpText, error),
    "aria-invalid": error ? ("true" as const) : undefined,
    "aria-label": label,
  };

  return (
    <FactoryGraphEditorAddField
      error={error}
      helpText={helpText}
      inputId={inputId}
      label={label}
    >
      {emptyOption ? (
        <OptionalEnumSelect
          {...ariaProps}
          emptyOptionLabel={emptyOption.label}
          id={inputId}
          onValueChange={(nextValue) => {
            onChange(nextValue ?? "");
          }}
          options={enumOptions}
          value={value || null}
        />
      ) : (
        <EnumSelect
          {...ariaProps}
          id={inputId}
          onValueChange={onChange}
          options={enumOptions}
          value={value}
        />
      )}
    </FactoryGraphEditorAddField>
  );
}

const factoryGraphAddPromptAutocompleteState = {
  message: "",
  status: "empty" as const,
};

export function FactoryGraphEditorPromptBodyField({
  helpText,
  label,
  loadingMessage,
  onChange,
  startupErrorMessage,
  value,
}: {
  helpText?: string;
  label: string;
  loadingMessage: string;
  onChange: (value: string) => void;
  startupErrorMessage: string;
  value: string;
}) {
  return (
    <FactoryGraphEditorAddField helpText={helpText} label={label}>
      <MonacoPromptEditor
        ariaLabel={label}
        autocompleteState={factoryGraphAddPromptAutocompleteState}
        className={"bg-surface"}
        loadingMessage={loadingMessage}
        modelPath={FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH}
        onChange={onChange}
        startupErrorMessage={startupErrorMessage}
        value={value}
      />
    </FactoryGraphEditorAddField>
  );
}
