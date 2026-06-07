import type { ReactNode } from "react";

import {
  FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
} from "../../../components/prompt-editor";
import {
  FormDescription,
  FormError,
  FormField,
  FormLabel,
  Input,
  NativeSelect,
  Textarea,
} from "../../../components/ui";

const FACTORY_GRAPH_ADD_INPUT_CLASS = "bg-surface";

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
      {helpText ? <FormDescription>{helpText}</FormDescription> : null}
      {error ? <FormError>{error}</FormError> : null}
    </FormField>
  );
}

export function FactoryGraphEditorTextareaField({
  error,
  inputId,
  label,
  onChange,
  value,
}: {
  error?: string;
  inputId: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <FactoryGraphEditorAddField error={error} inputId={inputId} label={label}>
      <Textarea
        aria-label={label}
        className={FACTORY_GRAPH_ADD_INPUT_CLASS}
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
        className={FACTORY_GRAPH_ADD_INPUT_CLASS}
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
  return (
    <FactoryGraphEditorAddField
      error={error}
      helpText={helpText}
      inputId={inputId}
      label={label}
    >
      <NativeSelect
        aria-label={label}
        className={FACTORY_GRAPH_ADD_INPUT_CLASS}
        id={inputId}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </NativeSelect>
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
        className={FACTORY_GRAPH_ADD_INPUT_CLASS}
        loadingMessage={loadingMessage}
        modelPath={FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH}
        onChange={onChange}
        startupErrorMessage={startupErrorMessage}
        value={value}
      />
    </FactoryGraphEditorAddField>
  );
}
