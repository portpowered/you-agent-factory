import { FactorySessionsAPIError } from "../../../api/factory-sessions";
import {
  factorySessionFieldTarget,
  factorySessionTargetTarget,
} from "../../../testing/factory-validation-target-fixtures";
import { getHeaderControlsMessages } from "../messages/header-controls";
import {
  classifyFactorySessionFolderValidationError,
  factorySessionTargetOptionValue,
  folderValidationStatusMessage,
  initNewFactoryNestedPath,
  isCanonicalNestedFactorySession,
  moveSessionTabOrder,
  normalizeFactorySessionsError,
  orderFactorySessions,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: keeps the utility contract coverage together in one focused spec block.
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

  it("prefers the selected folder identity for canonical nested init-new-factory sessions", () => {
    const nestedInitSession = {
      id: "session-nested-init",
      project: "my-project",
      factoryDir: "/workspace/my-project/factory",
      folderPath: "/workspace/my-project",
      target: { kind: "named", name: "factory" },
    } as const;

    expect(isCanonicalNestedFactorySession(nestedInitSession)).toBe(true);
    expect(sessionTabLabel(nestedInitSession)).toBe("my-project");
    expect(initNewFactoryNestedPath("/workspace/my-project")).toBe(
      "/workspace/my-project/factory",
    );
  });

  it("keeps named target labels for non-canonical nested factory sessions", () => {
    const namedNestedSession = {
      id: "session-review",
      project: "catalog",
      factoryDir: "/workspace/customers/northwind/examples/catalog/review",
      folderPath: "/workspace/customers/northwind/examples/catalog",
      target: { kind: "named", name: "review" },
    } as const;

    expect(isCanonicalNestedFactorySession(namedNestedSession)).toBe(false);
    expect(sessionTabLabel(namedNestedSession)).toBe("review");
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

  it("projects persisted session-tab order and moves dragged tabs by insertion index", () => {
    const alphaSession = {
      ...namedSession,
      id: "alpha",
      target: { kind: "named", name: "alpha" },
    } as const;
    const betaSession = {
      ...namedSession,
      id: "beta",
      target: { kind: "named", name: "beta" },
    } as const;
    const gammaSession = {
      ...namedSession,
      id: "gamma",
      target: { kind: "named", name: "gamma" },
    } as const;

    expect(
      orderFactorySessions(
        [alphaSession, betaSession, gammaSession],
        ["gamma", "alpha"],
      ).map((session) => session.id),
    ).toEqual(["gamma", "alpha", "beta"]);

    expect(moveSessionTabOrder(["alpha", "beta", "gamma"], "alpha", 2)).toEqual(
      ["beta", "alpha", "gamma"],
    );
    expect(moveSessionTabOrder(["alpha", "beta", "gamma"], "gamma", 0)).toEqual(
      ["gamma", "alpha", "beta"],
    );
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
        new FactorySessionsAPIError("config load failed", {
          code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
          targets: [
            factorySessionTargetTarget(
              "config_load_failed",
              "default",
              "broken factory config",
            ),
          ],
        }),
      ),
    ).toBe("config_load_failed");
    expect(
      folderValidationStatusMessage(
        { status: "error", reason: "config_load_failed" },
        messages,
      ),
    ).toBeNull();
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
