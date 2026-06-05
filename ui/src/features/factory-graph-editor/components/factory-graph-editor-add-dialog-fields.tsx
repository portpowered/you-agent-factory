import type { ReactNode } from "react";

import {
  FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
} from "../../../components/prompt-editor";
import { Input, Select, Textarea } from "../../../components/ui";

const FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS = "grid gap-2";
const FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS =
  "text-sm font-semibold text-on-surface";
const FACTORY_GRAPH_ADD_FIELD_HELP_CLASS =
  "m-0 text-xs leading-5 text-on-surface-variant";
const FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS =
  "m-0 text-sm text-on-error-container";
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
    <label className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS} htmlFor={inputId}>
      {label}
    </label>
  ) : (
    <p className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS}>{label}</p>
  );

  return (
    <div className={FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS}>
      {labelContent}
      {children}
      {helpText ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_HELP_CLASS}>{helpText}</p>
      ) : null}
      {error ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS}>{error}</p>
      ) : null}
    </div>
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
      <Select
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
      </Select>
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
