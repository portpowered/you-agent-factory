import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import { Button, Label, Text } from "@you-agent-factory/components/primitives";
import { cn } from "../../../../lib/cn";
import type { EditableWorkTypeValues } from "../../../current-factory-definition/lib/work-type-editable-values";
import { CurrentSelectionSectionHeader } from "../../base/components/layout/current-selection-section-header";
import { CurrentSelectionSupportingText } from "../../base/components/presentation/current-selection-supporting-text";
import { workStateGraphNodeId } from "../lib/work-state-graph-node-id";
import type { getWorkTypeDetailMessages } from "../messages/work-type-detail";

function WorkTypeStateRowContent({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  state: NonNullable<EditableWorkTypeValues["states"]>[number];
}) {
  return (
    <div className="grid min-w-0 gap-1">
      <div className="grid min-w-0 gap-1">
        <Label>{messages.stateNameColumnLabel}</Label>
        <Text as="span" className="min-w-0 [overflow-wrap:anywhere]">
          {state.name}
        </Text>
      </div>
      <div className="grid min-w-0 gap-1">
        <Label>{messages.stateTypeColumnLabel}</Label>
        <Text
          as="span"
          className="min-w-0 [overflow-wrap:anywhere]"
          variant="supporting"
        >
          {messages.localizeWorkStateType(state.type)}
        </Text>
      </div>
    </div>
  );
}

export function WorkTypeStatesList({
  messages,
  onSelectWorkStateGraphNode,
  states,
  workTypeName,
}: {
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  onSelectWorkStateGraphNode?: (graphNodeId: string) => void;
  states: EditableWorkTypeValues["states"];
  workTypeName: string;
}) {
  const headingId = "work-type-states-heading";

  return (
    <section aria-labelledby={headingId} className="mt-4 grid gap-2.5">
      <CurrentSelectionSectionHeader
        headingId={headingId}
        title={messages.statesHeading}
      />
      {states == null || states.length === 0 ? (
        <CurrentSelectionSupportingText>
          {messages.statesEmpty}
        </CurrentSelectionSupportingText>
      ) : (
        <ul className="m-0 grid list-none gap-2 p-0">
          {states.map((state) => {
            const graphNodeId = workStateGraphNodeId(workTypeName, state.name);

            return (
              <li key={`${state.name}:${state.type}`}>
                {onSelectWorkStateGraphNode ? (
                  <Button
                    aria-label={messages.selectWorkStateGraphNodeLabel(
                      state.name,
                    )}
                    className={cn(
                      surfacePanelVariants({
                        className: "grid min-w-0 gap-1 px-3 py-2",
                        radius: "lg",
                      }),
                      "h-auto min-h-0 w-full justify-start rounded-lg px-3 py-2 font-normal shadow-none",
                      "border-outline bg-surface-container-high text-left hover:border-outline-variant hover:bg-af-overlay",
                    )}
                    onClick={() => onSelectWorkStateGraphNode(graphNodeId)}
                    tone="outline"
                    type="button"
                  >
                    <WorkTypeStateRowContent
                      messages={messages}
                      state={state}
                    />
                  </Button>
                ) : (
                  <div
                    className={surfacePanelVariants({
                      className: "grid min-w-0 gap-1 px-3 py-2",
                      radius: "lg",
                    })}
                  >
                    <WorkTypeStateRowContent
                      messages={messages}
                      state={state}
                    />
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
