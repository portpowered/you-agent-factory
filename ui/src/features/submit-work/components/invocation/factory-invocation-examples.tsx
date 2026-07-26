import type { components } from "../../../../api/generated/openapi";
import { Text } from "@you-agent-factory/components/primitives";
import {
  AlertPanel,
  AlertPanelText,
} from "../../../../components/ui/alert-panel";

type FactoryInvocationExample =
  components["schemas"]["FactoryInvocationExample"];

export function FactoryInvocationExamples({
  examples,
  locale,
  title,
}: {
  examples: FactoryInvocationExample[];
  locale?: string;
  title: string;
}) {
  return (
    <AlertPanel compact role="status" tone="neutral" variant="empty">
      <AlertPanelText>{title}</AlertPanelText>
      <div className="grid gap-2 pt-1">
        {examples.map((example) => {
          const description = resolveNameValue(example.description, locale);
          return (
            <div className="grid gap-1" key={example.name}>
              <Text className="font-medium" variant="supporting">
                {example.name}
              </Text>
              {description ? (
                <Text className="text-on-surface-variant" variant="supporting">
                  {description}
                </Text>
              ) : null}
              {Object.keys(example.args).length > 0 ? (
                <Text className="font-mono text-xs" variant="supporting">
                  {JSON.stringify(example.args)}
                </Text>
              ) : null}
            </div>
          );
        })}
      </div>
    </AlertPanel>
  );
}

function resolveNameValue(
  nameValue: FactoryInvocationExample["description"],
  locale?: string,
): string {
  return (locale ? nameValue.values?.[locale] : undefined) ?? nameValue.value;
}
