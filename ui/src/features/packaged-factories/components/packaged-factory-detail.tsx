import { useId, useState } from "react";

import { Heading, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel, AlertPanelText } from "../../../components/ui/alert-panel";
import { CodePanel } from "../../../components/ui/code-panel";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import type {
  PackagedFactoryConfigurationFormat,
  PackagedFactoryDetailViewModel,
  PackagedFactoryInvocationExampleViewModel,
} from "../lib/projection";
import type { PackagedFactoryInventoryMessages } from "../messages/inventory";

export type PackagedFactoryCopyText = (value: string) => Promise<void>;

type CopyFeedback =
  | { readonly status: "error"; readonly message: string }
  | { readonly status: "success"; readonly message: string };

interface CopyActionProps {
  readonly accessibleName: string;
  readonly copyText: PackagedFactoryCopyText;
  readonly errorMessage: string;
  readonly successMessage: string;
  readonly value: string;
}

function CopyAction({
  accessibleName,
  copyText,
  errorMessage,
  successMessage,
  value,
}: CopyActionProps) {
  const [feedback, setFeedback] = useState<CopyFeedback>();

  async function copy() {
    try {
      await copyText(value);
      setFeedback({ status: "success", message: successMessage });
    } catch {
      setFeedback({ status: "error", message: errorMessage });
    }
  }

  return (
    <div className="grid justify-items-start gap-layout-tight">
      <DashboardActionButton
        aria-label={accessibleName}
        onClick={() => void copy()}
        type="button"
      >
        {accessibleName}
      </DashboardActionButton>
      {feedback ? (
        <AlertPanel
          className="min-w-0"
          compact
          role={feedback.status === "error" ? "alert" : "status"}
          tone={feedback.status === "error" ? "danger" : "success"}
        >
          <AlertPanelText>{feedback.message}</AlertPanelText>
        </AlertPanel>
      ) : null}
    </div>
  );
}

function InvocationExample({
  copyText,
  example,
  messages,
}: {
  readonly copyText: PackagedFactoryCopyText;
  readonly example: PackagedFactoryInvocationExampleViewModel;
  readonly messages: PackagedFactoryInventoryMessages;
}) {
  return (
    <li className="grid min-w-0 gap-layout-element">
      <Heading as="h5">{example.name}</Heading>
      <Text as="p">
        {example.description.status === "available"
          ? example.description.value
          : messages.descriptionUnavailable}
      </Text>
      <CodePanel maxHeight="sm">
        <code>{example.copyValue}</code>
      </CodePanel>
      <CopyAction
        accessibleName={messages.invocationCopyLabel(example.name)}
        copyText={copyText}
        errorMessage={messages.invocationCopyFailed}
        successMessage={messages.invocationCopied}
        value={example.copyValue}
      />
    </li>
  );
}

export function PackagedFactoryDetail({
  copyText,
  detail,
  headingID,
  messages,
}: {
  readonly copyText: PackagedFactoryCopyText;
  readonly detail: PackagedFactoryDetailViewModel;
  readonly headingID: string;
  readonly messages: PackagedFactoryInventoryMessages;
}) {
  const [format, setFormat] =
    useState<PackagedFactoryConfigurationFormat>("json");
  const configurationPanelID = useId();
  const configuration = detail.configurations[format];

  return (
    <article
      aria-labelledby={headingID}
      className="grid min-w-0 gap-layout-section"
    >
      <div className="grid min-w-0 gap-layout-element">
        <Heading as="h3" id={headingID}>
          {detail.stableName}
        </Heading>
        <Text as="p">
          {detail.description.status === "available"
            ? detail.description.value
            : messages.descriptionUnavailable}
        </Text>
        <Text as="p" variant="supporting">
          {messages.projectLabel}: {detail.project}
        </Text>
      </div>

      <section className="grid min-w-0 gap-layout-element">
        <Heading as="h4">{messages.configurationTitle}</Heading>
        <fieldset className="m-0 flex min-w-0 flex-wrap gap-layout-tight border-0 p-0">
          <legend className="sr-only">
            {messages.configurationFormatLabel}
          </legend>
          {detail.availableFormats.map((availableFormat) => (
            <DashboardActionButton
              aria-controls={configurationPanelID}
              aria-pressed={format === availableFormat}
              key={availableFormat}
              onClick={() => setFormat(availableFormat)}
              type="button"
            >
              {availableFormat.toUpperCase()}
            </DashboardActionButton>
          ))}
        </fieldset>
        <CodePanel id={configurationPanelID} maxHeight="lg">
          <code>{configuration.displayValue}</code>
        </CodePanel>
        <CopyAction
          accessibleName={messages.configurationCopyLabel(format.toUpperCase())}
          copyText={copyText}
          errorMessage={messages.configurationCopyFailed}
          successMessage={messages.configurationCopied}
          value={configuration.copyValue}
        />
      </section>

      <section className="grid min-w-0 gap-layout-element">
        <Heading as="h4">{messages.invocationExamplesTitle}</Heading>
        {detail.examples.status === "none" ? (
          <Text as="p" variant="supporting">
            {messages.noExamples}
          </Text>
        ) : (
          <ul className="grid min-w-0 gap-layout-section">
            {detail.examples.items.map((example) => (
              <InvocationExample
                copyText={copyText}
                example={example}
                key={example.name}
                messages={messages}
              />
            ))}
          </ul>
        )}
      </section>
    </article>
  );
}
