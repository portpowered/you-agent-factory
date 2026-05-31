import "../../../../testing/bun-factory-sessions-api-mocks";
import { FactorySessionsAPIError } from "../../../api/factory-sessions";
import { factorySessionFieldTarget } from "../../../testing/factory-validation-target-fixtures";
import { getHeaderControlsMessages } from "../messages/header-controls";
import {
  classifyFactorySessionFolderValidationError,
  factorySessionTargetOptionValue,
  folderValidationStatusMessage,
  normalizeFactorySessionsError,
  selectedFactorySessionTarget,
  sessionCloseLabel,
  sessionPanelID,
  sessionTabID,
  sessionTabLabel,
  sessionTabSecondaryPath,
} from "./dashboard-session-tabs-utils";

const messages = getHeaderControlsMessages("en");

const namedSession = {
  id: "session:/review",
  project: "project fallback",
  factoryDir: "/tmp/factory-dir",
  folderPath: "/tmp/folder-path",
  target: { kind: "named", name: "Review Factory" },
} as const;

const fallbackSession = {
  id: "session with spaces",
  project: "project fallback",
  factoryDir: "",
  folderPath: "/tmp/folder-path",
  target: { kind: "default" },
} as const;

const targets = [
  {
    label: "default",
    path: "/tmp/factory",
    ref: { kind: "default" },
  },
  {
    label: "review",
    path: "/tmp/factory/review",
    ref: { kind: "named", name: "review" },
  },
] as const;

describe("dashboard session tabs utils", () => {
  it("builds session labels and DOM ids from the best available session metadata", () => {
    expect(sessionTabLabel(namedSession)).toBe("Review Factory");
    expect(sessionCloseLabel(namedSession, messages)).toBe(
      "Close Review Factory session",
    );
    expect(sessionTabLabel(fallbackSession)).toBe("folder-path");
    expect(sessionTabID("session-tabs", namedSession.id)).toBe(
      "session-tabs-tab-session-review",
    );
    expect(sessionPanelID("session-tabs", fallbackSession.id)).toBe(
      "session-tabs-panel-session-with-spaces",
    );
  });

  it("shrinks long session-tab secondary paths by hiding the prefix", () => {
    expect(sessionTabSecondaryPath("/workspace/catalog")).toBe(
      "/workspace/catalog",
    );
    expect(
      sessionTabSecondaryPath(
        "/workspace/customers/northwind/agent-factory/examples/support",
        28,
      ),
    ).toBe("...-factory/examples/support");
    expect(
      sessionTabSecondaryPath(
        "/workspace/customers/northwind/agent-factory/examples/support",
        28,
      ),
    ).toHaveLength(28);
  });

  it("maps validation states and API errors to the visible folder-validation messages", () => {
    expect(
      folderValidationStatusMessage({ status: "idle" }, messages),
    ).toBeNull();
    expect(folderValidationStatusMessage({ status: "pending" }, messages)).toBe(
      messages.openSessionValidationPendingLabel,
    );
    expect(
      folderValidationStatusMessage(
        {
          status: "ready",
          targets: [targets[0]],
        },
        messages,
      ),
    ).toBe(messages.openSessionLaunchReadySingleTarget);
    expect(
      folderValidationStatusMessage(
        {
          status: "ready",
          targets: [...targets],
        },
        messages,
      ),
    ).toBe(messages.openSessionLaunchReadyMultipleTargets);
    expect(
      folderValidationStatusMessage(
        { status: "error", reason: "target_not_found" },
        messages,
      ),
    ).toBe(messages.openSessionOverrideNotFoundError);

    expect(
      classifyFactorySessionFolderValidationError(
        new FactorySessionsAPIError("ignored", {
          code: "BAD_REQUEST",
          targets: [
            factorySessionFieldTarget("missing", "folderPath", "ignored"),
          ],
        }),
      ),
    ).toBe("missing");
    expect(
      classifyFactorySessionFolderValidationError(
        new FactorySessionsAPIError(
          'factory session target "missing" was not found',
          { code: "BAD_REQUEST" },
        ),
      ),
    ).toBe("target_not_found");
    expect(
      classifyFactorySessionFolderValidationError(
        new FactorySessionsAPIError(
          "stat factory session folder: permission denied",
          { code: "BAD_REQUEST" },
        ),
      ),
    ).toBe("unreadable");
  });

  it("normalizes selection helpers and wraps unknown factory-session errors", () => {
    expect(factorySessionTargetOptionValue(targets[0])).toBe("default");
    expect(factorySessionTargetOptionValue(targets[1])).toBe("named:review");
    expect(selectedFactorySessionTarget([...targets], "named:review")).toEqual(
      targets[1],
    );
    expect(
      selectedFactorySessionTarget([...targets], "named:missing"),
    ).toBeNull();

    const wrappedError = normalizeFactorySessionsError("boom");
    expect(wrappedError).toBeInstanceOf(FactorySessionsAPIError);
    expect(wrappedError.code).toBe("INTERNAL_ERROR");
    expect(wrappedError.responseBody).toBe("boom");
  });
});
