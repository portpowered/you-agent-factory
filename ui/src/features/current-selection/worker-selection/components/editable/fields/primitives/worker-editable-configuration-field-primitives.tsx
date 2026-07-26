import { OptionalEnumSelect } from "@you-agent-factory/components/forms";
import type { ReactNode } from "react";

import { Label, Text } from "@you-agent-factory/components/primitives";
import { FormWarning } from "../../../../../../../components/ui/form-field";
import { CurrentSelectionDetailFeedback } from "../../../../../base/components/detail/current-selection-detail-feedback";
import { CurrentSelectionFormField } from "../../../../../base/components/layout/current-selection-form-layout";
import type { EditableWorkerOverwriteField } from "../../../../lib/detail-card-types";
import type {
  ReadyWorkerEditableConfigurationState,
  WorkerEditableConfigurationMessages,
} from "./worker-editable-configuration-field-types";

export function WorkerEditableConfigurationField({
  errorMessage,
  fieldId,
  input,
  label,
  supportingContent,
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
  supportingContent?: ReactNode;
}) {
  return (
    <CurrentSelectionFormField>
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <CurrentSelectionDetailFeedback id={`${fieldId}-error`} tone="danger">
          {errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
    </CurrentSelectionFormField>
  );
}

export function WorkerEditableConfigurationFieldHelp({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <Text className="m-0 text-on-surface-subtle" variant="supporting">
      {children}
    </Text>
  );
}

export function WorkerEditableConfigurationServerChangedHint({
  fieldName,
  messages,
  state,
}: {
  fieldName: EditableWorkerOverwriteField;
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
}) {
  if (!state.overwriteFieldNames.includes(fieldName)) {
    return null;
  }

  return (
    <FormWarning>
      {messages.editableConfigurationServerFieldChangedHint}
    </FormWarning>
  );
}

export function WorkerOptionalEnumSelect<T extends string>({
  ariaDescribedBy,
  ariaInvalid,
  id,
  label,
  notConfiguredLabel,
  onChange,
  options,
  renderLabel,
  value,
}: {
  ariaDescribedBy?: string;
  ariaInvalid?: boolean;
  id: string;
  label: string;
  notConfiguredLabel: string;
  onChange: (value: T | null) => void;
  options: readonly T[];
  renderLabel: (value: string) => string;
  value: T | null;
}) {
  return (
    <OptionalEnumSelect
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid ? "true" : undefined}
      aria-label={label}
      emptyOptionLabel={notConfiguredLabel}
      id={id}
      onValueChange={(nextValue) => onChange(nextValue as T | null)}
      options={options.map((option) => ({
        label: renderLabel(option),
        value: option,
      }))}
      value={value}
    />
  );
}
