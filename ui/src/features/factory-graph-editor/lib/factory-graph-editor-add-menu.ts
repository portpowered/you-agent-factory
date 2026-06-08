import type { FactoryGraphEditorMenuAction } from "../components/controls/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import type { CanonicalFactoryDefinition } from "./draft/factory-graph-draft-types";

export function buildFactoryGraphAddEntityMenuActions(
  factoryDefinition: CanonicalFactoryDefinition | null,
  locale?: string | null,
): FactoryGraphEditorMenuAction[] {
  const hasWorkTypes = (factoryDefinition?.workTypes?.length ?? 0) > 0;
  const messages = getFactoryGraphEditorMessages(locale);

  return [
    {
      description: messages.addMenuAction("doc").description,
      id: "doc",
      label: messages.addMenuAction("doc").label,
    },
    {
      description: messages.addMenuAction("workstation").description,
      id: "workstation",
      label: messages.addMenuAction("workstation").label,
    },
    {
      description: messages.addMenuAction("worker").description,
      id: "worker",
      label: messages.addMenuAction("worker").label,
    },
    {
      description: messages.addMenuAction("work-type").description,
      id: "work-type",
      label: messages.addMenuAction("work-type").label,
    },
    {
      description: messages.addMenuAction("work-state").description,
      disabled: !hasWorkTypes,
      id: "work-state",
      label: messages.addMenuAction("work-state").label,
    },
    {
      description: messages.addMenuAction("resource").description,
      id: "resource",
      label: messages.addMenuAction("resource").label,
    },
  ];
}
