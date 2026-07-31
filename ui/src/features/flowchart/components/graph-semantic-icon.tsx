import {
  GRAPH_SEMANTIC_ICON_KINDS,
  GraphSemanticIcon as FactoryGraphSemanticIcon,
  type GraphSemanticIconKind,
  type GraphSemanticIconProps,
} from "@you-agent-factory/factory-graph";

import { getActivityGraphMessages } from "../messages/activity-graph";

export { GRAPH_SEMANTIC_ICON_KINDS };
export type { GraphSemanticIconKind, GraphSemanticIconProps };

export function graphSemanticIconLabel(
  kind: GraphSemanticIconProps["kind"],
  locale?: string | null,
): string {
  const messages = getActivityGraphMessages(locale);

  return GRAPH_SEMANTIC_ICON_KINDS.includes(kind as GraphSemanticIconKind)
    ? messages.graphSemanticIconLabel(kind as GraphSemanticIconKind)
    : messages.unknownGraphSemanticIconLabel;
}

/** Website localization adapter for the canonical Factory graph semantic icon. */
export function GraphSemanticIcon(props: GraphSemanticIconProps) {
  return (
    <FactoryGraphSemanticIcon
      {...props}
      label={props.label ?? graphSemanticIconLabel(props.kind, props.locale)}
    />
  );
}
