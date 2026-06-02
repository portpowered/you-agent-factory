import {
  FACTORY_GRAPH_ADD_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
} from "../../../components/prompt-editor";
import { Input, Select, Textarea } from "../../../components/ui";

export const FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS = "grid gap-2";
export const FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS =
  "text-sm font-semibold text-af-text";
export const FACTORY_GRAPH_ADD_FIELD_HELP_CLASS =
  "m-0 text-xs leading-5 text-af-text-muted";
export const FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS =
  "m-0 text-sm text-af-danger-text";
export const FACTORY_GRAPH_ADD_INPUT_CLASS = "bg-af-surface";

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
    <div className={FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS}>
      <label className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
      <Textarea
        aria-label={label}
        className={FACTORY_GRAPH_ADD_INPUT_CLASS}
        id={inputId}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      />
      {error ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS}>{error}</p>
      ) : null}
    </div>
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
    <div className={FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS}>
      <label className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
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
      {helpText ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_HELP_CLASS}>{helpText}</p>
      ) : null}
      {error ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS}>{error}</p>
      ) : null}
    </div>
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
    <div className={FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS}>
      <label className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
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
      {helpText ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_HELP_CLASS}>{helpText}</p>
      ) : null}
      {error ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_ERROR_CLASS}>{error}</p>
      ) : null}
    </div>
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
    <div className={FACTORY_GRAPH_ADD_FIELD_GROUP_CLASS}>
      <p className={FACTORY_GRAPH_ADD_FIELD_LABEL_CLASS}>{label}</p>
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
      {helpText ? (
        <p className={FACTORY_GRAPH_ADD_FIELD_HELP_CLASS}>{helpText}</p>
      ) : null}
    </div>
  );
}
