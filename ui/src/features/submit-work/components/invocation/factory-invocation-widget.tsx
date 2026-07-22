// biome-ignore-all lint/style/noExcessiveLinesPerFile: invocation widget keeps the generated form controls colocated with the widget shell.
import { OptionalEnumSelect } from "@you-agent-factory/components/forms";
import { Plus, X } from "lucide-react";
import type { ReactNode } from "react";
import type { SessionFactoryInvocationResponse } from "../../../../api/session-factory";
import {
  AlertPanel,
  AlertPanelText,
  Button,
  DashboardIconButtonShell,
  FormError,
  Input,
  Label,
  Text,
} from "../../../../components/ui";
import { DashboardWidgetFrame } from "../../../bento/public";
import { useFactoryInvocationWidget } from "../../hooks/use-factory-invocation-widget";
import type { InvocationFieldModel } from "../../lib/factory-invocation-form";
import { getSubmitWorkMessages } from "../../messages/submit-work";
import { FactoryInvocationExamples } from "./factory-invocation-examples";

export interface FactoryInvocationWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  sessionID: string | null | undefined;
  widgetId?: string;
}

export function FactoryInvocationWidget({
  headerAction,
  locale,
  sessionID,
  widgetId = "submit-work",
}: FactoryInvocationWidgetProps) {
  const messages = getSubmitWorkMessages(locale);
  const {
    currentFactory,
    fieldErrors,
    fieldValues,
    isSubmitting,
    projection,
    setBooleanValue,
    setFieldValue,
    setRepeatedFieldValue,
    status,
    submit,
  } = useFactoryInvocationWidget(sessionID, messages);

  if (currentFactory.isLoading) {
    return (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title={messages.invocation.cardTitle}
        widgetId={widgetId}
      >
        <InvocationStatusPanel
          status={{
            kind: "submitting",
            message: messages.invocation.loadingState,
          }}
          widgetId={widgetId}
        />
      </DashboardWidgetFrame>
    );
  }

  if (currentFactory.error) {
    return (
      <DashboardWidgetFrame
        headerAction={headerAction}
        title={messages.invocation.cardTitle}
        widgetId={widgetId}
      >
        <InvocationStatusPanel
          status={{
            kind: "error",
            message: currentFactory.error.message,
          }}
          widgetId={widgetId}
        />
      </DashboardWidgetFrame>
    );
  }

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
      title={messages.invocation.cardTitle}
      widgetId={widgetId}
    >
      <form
        className="grid gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <div className="grid gap-4" data-submit-work-primary-content="">
          {projection.fields.length === 0 ? (
            <AlertPanel compact role="status" tone="neutral" variant="empty">
              <AlertPanelText>
                {messages.invocation.emptyParametersState}
              </AlertPanelText>
            </AlertPanel>
          ) : (
            projection.fields.map((field) => (
              <InvocationField
                error={fieldErrors[field.name]}
                field={field}
                key={field.name}
                messages={messages}
                onBooleanValueChange={setBooleanValue}
                onRepeatedValueChange={setRepeatedFieldValue}
                onValueChange={setFieldValue}
                value={fieldValues[field.name] ?? []}
                widgetId={widgetId}
              />
            ))
          )}
          {projection.outputContract ? (
            <InvocationOutputHint
              messages={messages}
              outputContract={projection.outputContract}
            />
          ) : null}
          {projection.examples.length > 0 ? (
            <FactoryInvocationExamples
              examples={projection.examples}
              locale={locale}
              title={messages.invocation.examplesTitle}
            />
          ) : null}
        </div>
        <div className="grid gap-3">
          {status.kind !== "idle" && status.message ? (
            <InvocationStatusPanel
              status={{
                kind: status.kind,
                message: status.message,
              }}
              widgetId={widgetId}
            />
          ) : null}
          {status.response?.primaryResult ? (
            <InvocationPrimaryResult
              messages={messages}
              response={status.response}
            />
          ) : null}
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            className="w-full justify-center"
            type="submit"
          >
            {isSubmitting
              ? messages.invocation.submittingAction
              : messages.invocation.submitAction}
          </Button>
        </div>
      </form>
    </DashboardWidgetFrame>
  );
}

function InvocationField({
  error,
  field,
  messages,
  onBooleanValueChange,
  onRepeatedValueChange,
  onValueChange,
  value,
  widgetId,
}: {
  error?: string;
  field: InvocationFieldModel;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onBooleanValueChange: (
    name: string,
    value: "false" | "true" | undefined,
  ) => void;
  onRepeatedValueChange: (name: string, values: string[]) => void;
  onValueChange: (name: string, value: string) => void;
  value: string[];
  widgetId: string;
}) {
  const fieldID = `${widgetId}-${field.name}`;
  const errorID = `${fieldID}-error`;
  const descriptionID = `${fieldID}-description`;
  const labelID = `${fieldID}-label`;
  const controlID =
    field.kind === "repeated" ? `${widgetId}-${field.name}-0` : fieldID;
  const helperLines = buildFieldHelperLines(field, messages);
  const describedBy = [
    helperLines.length > 0 ? descriptionID : null,
    error ? errorID : null,
  ]
    .filter((entry): entry is string => entry !== null)
    .join(" ");
  const labelContent = (
    <Label>
      {field.label}{" "}
      {field.required ? (
        <Text
          as="span"
          className="text-on-error-container"
          variant="supporting"
        >
          ({messages.invocation.requiredAffordance})
        </Text>
      ) : null}
    </Label>
  );

  return (
    <div className="grid gap-2">
      {field.kind === "boolean" ? (
        <fieldset
          aria-describedby={describedBy || undefined}
          className="m-0 grid gap-2 border-0 p-0"
        >
          <legend className="contents" id={labelID}>
            {labelContent}
          </legend>
          <div className="flex flex-wrap gap-2">
            <BooleanChoiceButton
              isActive={value[0] === "true"}
              label={messages.invocation.booleanTrueAction}
              onClick={() => onBooleanValueChange(field.name, "true")}
            />
            <BooleanChoiceButton
              isActive={value[0] === "false"}
              label={messages.invocation.booleanFalseAction}
              onClick={() => onBooleanValueChange(field.name, "false")}
            />
            {!field.required ? (
              <BooleanChoiceButton
                isActive={value.length === 0}
                label={messages.invocation.booleanUnsetAction}
                onClick={() => onBooleanValueChange(field.name, undefined)}
              />
            ) : null}
          </div>
        </fieldset>
      ) : null}
      {field.kind !== "boolean" ? (
        <>
          <label htmlFor={controlID} id={labelID}>
            {labelContent}
          </label>
          {field.kind === "choice" ? (
            <OptionalEnumSelect
              aria-describedby={describedBy || undefined}
              aria-invalid={error ? "true" : undefined}
              emptyOptionLabel={messages.invocation.selectOptionPlaceholder}
              id={controlID}
              onValueChange={(nextValue) =>
                onValueChange(field.name, nextValue ?? "")
              }
              options={field.choices.map((choice) => ({
                label: choice,
                value: choice,
              }))}
              value={value[0] ?? null}
            />
          ) : null}
          {field.kind === "repeated" ? (
            <RepeatedFieldEditor
              field={field}
              messages={messages}
              onValueChange={(values) =>
                onRepeatedValueChange(field.name, values)
              }
              value={value}
              widgetId={widgetId}
            />
          ) : null}
          {field.kind === "text" ? (
            <Input
              aria-describedby={describedBy || undefined}
              aria-invalid={error ? "true" : undefined}
              id={controlID}
              onChange={(event) =>
                onValueChange(field.name, event.target.value)
              }
              placeholder={inputPlaceholder(field, messages)}
              type="text"
              value={value[0] ?? ""}
            />
          ) : null}
        </>
      ) : null}
      {helperLines.length > 0 ? (
        <div className="grid gap-1" id={descriptionID}>
          {helperLines.map((line) => (
            <Text
              className="text-on-surface-variant"
              key={line}
              variant="supporting"
            >
              {line}
            </Text>
          ))}
        </div>
      ) : null}
      {error ? <FormError id={errorID}>{error}</FormError> : null}
    </div>
  );
}

function RepeatedFieldEditor({
  field,
  messages,
  onValueChange,
  value,
  widgetId,
}: {
  field: InvocationFieldModel;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onValueChange: (values: string[]) => void;
  value: string[];
  widgetId: string;
}) {
  const rows = value.length > 0 ? value : [""];

  return (
    <div className="grid gap-2">
      {rows.map((entry, index) => (
        <div
          className="flex items-center gap-2"
          key={repeatedRowIndexKey(field.name, index)}
        >
          <Input
            aria-label={field.label}
            id={`${widgetId}-${field.name}-${index}`}
            onChange={(event) => {
              const next = [...rows];
              next[index] = event.target.value;
              onValueChange(next);
            }}
            placeholder={inputPlaceholder(field, messages)}
            type="text"
            value={entry}
          />
          <DashboardIconButtonShell
            aria-label={messages.invocation.removeRepeatedValue(
              field.label,
              index + 1,
            )}
            onClick={() => {
              const next = rows.filter((_, rowIndex) => rowIndex !== index);
              onValueChange(next.length > 0 ? next : [""]);
            }}
            tone="outline"
            type="button"
          >
            <X aria-hidden="true" className="size-4" strokeWidth={1.8} />
          </DashboardIconButtonShell>
        </div>
      ))}
      <div className="flex justify-start">
        <Button
          className="justify-start"
          onClick={() => onValueChange([...rows, ""])}
          tone="outline"
          type="button"
        >
          <Plus aria-hidden="true" className="size-4" strokeWidth={1.8} />
          {messages.invocation.addRepeatedValue(field.label)}
        </Button>
      </div>
    </div>
  );
}

function InvocationOutputHint({
  messages,
  outputContract,
}: {
  messages: ReturnType<typeof getSubmitWorkMessages>;
  outputContract: NonNullable<
    ReturnType<
      typeof useFactoryInvocationWidget
    >["projection"]["outputContract"]
  >;
}) {
  const detailLines = [
    outputContract.description,
    outputContract.mode
      ? messages.invocation.outputModeLabel(outputContract.mode)
      : undefined,
    outputContract.pathParameter
      ? messages.invocation.outputPathParameter(outputContract.pathParameter)
      : undefined,
    outputContract.contentType
      ? messages.invocation.outputContentType(outputContract.contentType)
      : undefined,
    outputContract.fileExtension
      ? messages.invocation.outputFileExtension(outputContract.fileExtension)
      : undefined,
  ].filter(
    (entry): entry is string => typeof entry === "string" && entry.length > 0,
  );

  return (
    <AlertPanel compact role="status" tone="info" variant="empty">
      <AlertPanelText>{messages.invocation.outputHintTitle}</AlertPanelText>
      <div className="grid gap-1 pt-1">
        {detailLines.map((line) => (
          <Text
            className="text-on-surface-variant"
            key={line}
            variant="supporting"
          >
            {line}
          </Text>
        ))}
      </div>
    </AlertPanel>
  );
}

function InvocationPrimaryResult({
  messages,
  response,
}: {
  messages: ReturnType<typeof getSubmitWorkMessages>;
  response: SessionFactoryInvocationResponse;
}) {
  const parts = response.primaryResult ?? [];
  const textParts = parts
    .map((part) =>
      "text" in part && typeof part.text === "string" ? part.text : null,
    )
    .filter((part): part is string => part !== null);

  if (textParts.length === 0) {
    return (
      <AlertPanel compact role="status" tone="success" variant="empty">
        <AlertPanelText>
          {messages.invocation.primaryResultReady}
        </AlertPanelText>
      </AlertPanel>
    );
  }

  return (
    <AlertPanel compact role="status" tone="success" variant="empty">
      <AlertPanelText>{messages.invocation.primaryResultReady}</AlertPanelText>
      <div className="grid gap-2 pt-2">
        {textParts.map((part) => (
          <pre
            className="max-h-56 overflow-y-auto rounded-xl border border-outline bg-surface-container-high p-3 text-xs text-on-surface"
            key={`${response.traceId}:${part}`}
          >
            {part}
          </pre>
        ))}
      </div>
    </AlertPanel>
  );
}

function InvocationStatusPanel({
  status,
  widgetId,
}: {
  status: {
    kind: "error" | "submitting" | "success" | "validation-error";
    message?: string;
  };
  widgetId: string;
}) {
  const tone =
    status.kind === "error" || status.kind === "validation-error"
      ? "danger"
      : status.kind === "success"
        ? "success"
        : "info";

  return (
    <AlertPanel
      compact
      id={`${widgetId}-invocation-status`}
      role={tone === "danger" ? "alert" : "status"}
      tone={tone}
      variant="empty"
    >
      <AlertPanelText>{status.message}</AlertPanelText>
    </AlertPanel>
  );
}

function BooleanChoiceButton({
  isActive,
  label,
  onClick,
}: {
  isActive: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <Button
      aria-pressed={isActive}
      className="min-w-24"
      onClick={onClick}
      tone={isActive ? "default" : "outline"}
      type="button"
    >
      {label}
    </Button>
  );
}

function buildFieldHelperLines(
  field: InvocationFieldModel,
  messages: ReturnType<typeof getSubmitWorkMessages>,
): string[] {
  const lines = [
    field.description,
    field.position
      ? messages.invocation.positionalBinding(field.position)
      : undefined,
    field.hasNamedBinding
      ? messages.invocation.namedBinding(field.externalName ?? field.name)
      : undefined,
    field.aliases.length > 0
      ? messages.invocation.aliases(field.aliases)
      : undefined,
    field.hasStdinBinding ? messages.invocation.stdinBinding : undefined,
    field.defaultValues.length > 0
      ? messages.invocation.defaultValue(field.defaultValues)
      : undefined,
  ];

  return lines.filter(
    (line): line is string => typeof line === "string" && line.length > 0,
  );
}

function inputPlaceholder(
  field: InvocationFieldModel,
  messages: ReturnType<typeof getSubmitWorkMessages>,
): string {
  switch (field.pathHint) {
    case "directory":
      return messages.invocation.directoryPathPlaceholder;
    case "file":
      return messages.invocation.filePathPlaceholder;
    case "path":
      return messages.invocation.pathPlaceholder;
    default:
      return messages.invocation.textPlaceholder;
  }
}

function repeatedRowIndexKey(name: string, index: number): string {
  return `${name}-${index}`;
}
