import { afterEach, beforeEach } from "vitest";

import {
  assertStrictConsoleClean,
  installStrictConsoleGuard,
} from "./strict-console-guard";

const APP_SHELL_TEST_PATTERN = /\/src\/App\.[^/]+\.test\.tsx$/;
const CURRENT_SELECTION_TEST_PATTERN =
  /\/features\/current-selection\/(?!hooks\/).*\.test\.(tsx|ts)$/;

const MANUAL_GUARDED_SUITE_FILES = [
  "current-selection-widget.graph-draft-conflict-notifications.test.tsx",
  "current-selection-detail-layout.test.tsx",
] as const;

function isGuardedSuiteTestFile(filePath: string): boolean {
  if (filePath.includes("/testing/strict-console-guard.test.")) {
    return false;
  }

  if (
    MANUAL_GUARDED_SUITE_FILES.some((fileName) => filePath.endsWith(fileName))
  ) {
    return false;
  }

  return (
    APP_SHELL_TEST_PATTERN.test(filePath) ||
    CURRENT_SELECTION_TEST_PATTERN.test(filePath)
  );
}

let disposeGuard: (() => void) | undefined;

beforeEach((context) => {
  if (process.env.VITEST_DISABLE_GUARDED_CONSOLE === "1") {
    return;
  }

  const filePath = context.task.file.filepath;
  if (!isGuardedSuiteTestFile(filePath)) {
    return;
  }

  disposeGuard = installStrictConsoleGuard();
});

afterEach((context) => {
  if (process.env.VITEST_DISABLE_GUARDED_CONSOLE === "1") {
    return;
  }

  const filePath = context.task.file.filepath;
  if (!isGuardedSuiteTestFile(filePath)) {
    return;
  }

  try {
    assertStrictConsoleClean();
  } finally {
    disposeGuard?.();
    disposeGuard = undefined;
  }
});
