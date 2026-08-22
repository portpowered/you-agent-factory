import "@xyflow/react/dist/style.css";

import { Heading } from "@you-agent-factory/components/primitives";
import { useId } from "react";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { ImportFactoryValue } from "../../../api/session-factory";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { cn } from "../../../lib/cn";
import type { ReadFactoryImportFile } from "../../import/hooks/use-factory-png-drop";
import type { FactoryImportConfirmInput } from "../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import type { CurrentActivityGraphEditorController } from "../hooks/current-activity-graph-state-value";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import {
  type CurrentActivityGraphSaveNotificationEffects,
  CurrentActivityGraphSaveNotifications,
} from "./react-flow-current-activity-card-save-notifications";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

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
      <Heading as="h2" className="m-0" id={headingID}>
        {messages.title}
      </Heading>
    </div>
  );
}

function useCurrentActivityAccessibilityIDs(widgetInstanceID?: string) {
  const fallbackID = useId();

  return {
    headingID: `workflow-graph-heading-${widgetInstanceID ?? fallbackID}`,
  };
}

export interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (
    input: FactoryImportConfirmInput,
  ) => Promise<ImportFactoryValue>;
  editorController?: CurrentActivityGraphEditorController;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectDoc: (targetPath: string) => void;
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
  saveNotificationEffects?: CurrentActivityGraphSaveNotificationEffects;
  selection: CurrentActivitySelection | null;
  sessionID?: string | null;
  snapshot: DashboardSnapshot;
  widgetInstanceID?: string;
}

export function ReactFlowCurrentActivityCardView(
  props: ReactFlowCurrentActivityCardProps & {
    viewModel: CurrentActivityGraphCardViewModel;
    chromeless?: boolean;
    showHeaderActions?: boolean;
  },
) {
  const { viewModel, chromeless = false, showHeaderActions = false } = props;
  const editorController = props.editorController ?? viewModel;
  const { headingID } = useCurrentActivityAccessibilityIDs(
    props.widgetInstanceID,
  );
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    currentFactoryDefinition: props.snapshot.factory,
    locale: props.locale,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
    sessionID: props.sessionID,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;
  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;
  const handleDiscardEditorChanges = () => {
    editorController.leaveControls.discardChanges();
  };

  return (
    <DashboardPanelShell
      aria-labelledby={headingID}
      className={cn(
        "relative flex h-full max-h-full min-h-0 min-w-0 flex-col overflow-hidden",
        chromeless && "rounded-none border-0 bg-transparent shadow-none",
      )}
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
    >
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
        viewModel={viewModel}
        editorController={editorController}
        headingID={headingID}
        imports={imports}
        locale={props.locale}
        selection={props.selection}
        snapshot={props.snapshot}
      />
      <CurrentActivityGraphSaveNotifications
        editorController={editorController}
        locale={props.locale}
        notificationEffects={props.saveNotificationEffects}
      />
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName={props.snapshot.factory?.name ?? "factory"}
        discardEditorChanges={handleDiscardEditorChanges}
        editorController={editorController}
        imports={imports}
        locale={props.locale}
        readyImportPreviewState={readyImportPreviewState}
        shouldRenderImportPreviewDialog={shouldRenderImportPreviewDialog}
      />
    </DashboardPanelShell>
  );
}
