import "@xyflow/react/dist/style.css";

import { useId } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { ImportFactoryValue } from "../../../api/session-factory";
import { DashboardHeading } from "../../../components/ui";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { GraphEditorPlacementProvider } from "./graph-editor-placement-context";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

export { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";

export type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";

function CurrentActivityCardHeading({
  headingID,
  hidden = false,
  locale,
}: {
  headingID: string;
  hidden?: boolean;
  locale?: string;
}) {
  const messages = getWorkflowActivityShellMessages(locale);

  if (hidden) {
    return (
      <span className="sr-only" id={headingID}>
        {messages.title}
      </span>
    );
  }

  return (
    <div>
      <DashboardHeading as="h2" className="m-0" id={headingID}>
        {messages.title}
      </DashboardHeading>
    </div>
  );
}

function useCurrentActivityAccessibilityIDs(widgetInstanceID?: string) {
  const fallbackID = useId();

  return {
    headingID: `workflow-graph-heading-${widgetInstanceID ?? fallbackID}`,
  };
}

interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (
    input: FactoryImportConfirmInput,
  ) => Promise<ImportFactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectResource: (resourceName: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  widgetInstanceID?: string;
}

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const { sessionID } = useDashboardSession();
  const editor = useCurrentActivityGraphEditor(
    props.snapshot,
    props.locale,
    sessionID,
  );
  return (
    <ReactFlowCurrentActivityCardView
      {...props}
      editor={editor}
      showHeaderActions
    />
  );
}

export function ReactFlowCurrentActivityCardView(
  props: ReactFlowCurrentActivityCardProps & {
    editor: ReturnType<typeof useCurrentActivityGraphEditor>;
    showHeaderActions?: boolean;
  },
) {
  const { editor, showHeaderActions = false } = props;
  const { headingID } = useCurrentActivityAccessibilityIDs(
    props.widgetInstanceID,
  );
  const graph = useCurrentActivityGraphViewModel({ ...props, editor });
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    locale: props.locale,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;
  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;

  return (
    <GraphEditorPlacementProvider>
      <DashboardPanelShell
        aria-labelledby={headingID}
        className="relative flex h-full max-h-full min-h-0 min-w-0 flex-col overflow-hidden"
        style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
      >
        {showHeaderActions ? (
          <div className="mb-4">
            <CurrentActivityGraphHeaderActions
              editorMode={editor.editorMode}
              editorUnavailableClassifierWorkstationName={
                editor.editorUnavailableClassifierWorkstationName
              }
              hasChanges={editor.draftState.hasChanges}
              isDefinitionLoading={
                editor.editableDefinitionQuery.status === "pending"
              }
              loadErrorMessage={editor.editableDefinitionQuery.error?.message}
              locale={props.locale}
              onToggle={editor.handleEditorModeToggle}
            />
          </div>
        ) : null}
        {showHeaderActions ? (
          <CurrentActivityCardHeading
            headingID={headingID}
            locale={props.locale}
          />
        ) : (
          <CurrentActivityCardHeading
            headingID={headingID}
            hidden
            locale={props.locale}
          />
        )}
        <CurrentActivityGraphSurface
          editor={editor}
          graph={graph}
          headingID={headingID}
          imports={imports}
          locale={props.locale}
          selection={props.selection}
          snapshot={props.snapshot}
        />
        <CurrentActivityGraphSaveNotifications
          editor={editor}
          locale={props.locale}
        />
        <CurrentActivityGraphEditorDialogs
          currentSessionFactoryName={props.snapshot.factory?.name ?? "factory"}
          editor={editor}
          imports={imports}
          locale={props.locale}
          readyImportPreviewState={readyImportPreviewState}
          shouldRenderImportPreviewDialog={shouldRenderImportPreviewDialog}
        />
      </DashboardPanelShell>
    </GraphEditorPlacementProvider>
  );
}
