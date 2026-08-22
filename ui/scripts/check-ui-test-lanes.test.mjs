import { expect, it } from "vitest";

import {
  auditUiSourceFile,
  auditUiTestFile,
  classifiedUiTestLane,
} from "./check-ui-test-lanes.mjs";

it.each([
  ["features/example/lib/value.bun.unit.test.ts", "bun-unit"],
  ["features/example/lib/value.unit.test.ts", "unit"],
  ["features/example/lib/value.unit.test.mts", "unit"],
  ["features/example/lib/legacy.test.ts", "unit"],
  ["features/example/components/legacy.test.tsx", "component"],
  ["features/example/components/card.component.test.tsx", "component"],
  ["features/example/components/card.bun.component.test.tsx", "component"],
  ["features/example/lib/layout.performance.test.ts", "performance"],
  ["features/example/performance/layout.test.ts", "performance"],
  ["integration/example.browser.test.mjs", "browser"],
])("classifies %s as %s", (relativePath, expected) => {
  expect(classifiedUiTestLane(relativePath)).toBe(expected);
});

it("leaves malformed Bun unit names unowned for an actionable audit error", () => {
  expect(
    classifiedUiTestLane("features/example/lib/value.bun.unit.test.tsx"),
  ).toBe(null);
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.bun.unit.test.tsx",
      source: "",
    }),
  ).toEqual([
    "features/example/lib/value.bun.unit.test.tsx: Bun-native unit tests must use the exact .bun.unit.test.ts suffix; this path is ambiguous or unowned",
  ]);
});

it("rejects aggregate feature public imports", () => {
  expect(
    auditUiSourceFile({
      relativePath: "features/example/card.tsx",
      source: 'import { useFactoryTimelineStore } from "../timeline/public";',
    }),
  ).toEqual([
    "features/example/card.tsx: import the owning module instead of an aggregate feature public barrel",
  ]);
});

it("rejects a sibling public barrel import", () => {
  expect(
    auditUiSourceFile({
      relativePath: "features/example/card.test.tsx",
      source: 'import { ExampleCard } from "../public";',
    }),
  ).toEqual([
    "features/example/card.test.tsx: import the owning module instead of an aggregate feature public barrel",
  ]);
});

it("rejects general UI barrels in application code", () => {
  expect(
    auditUiSourceFile({
      relativePath: "features/example/card.tsx",
      source: 'import { Button } from "../../../components/ui";',
    }),
  ).toEqual([
    "features/example/card.tsx: import a focused UI module instead of the general components/ui barrel",
  ]);
  expect(
    auditUiSourceFile({
      relativePath: "features/example/card.tsx",
      source: 'import { Button } from "@you-agent-factory/components";',
    }),
  ).toEqual([
    "features/example/card.tsx: import a focused @you-agent-factory/components subpath instead of its package root",
  ]);
});

it("allows package-root imports in explicit package contract tests", () => {
  expect(
    auditUiSourceFile({
      relativePath: "components/ui/package-exports.test.tsx",
      source: 'import { Button } from "@you-agent-factory/components";',
    }),
  ).toEqual([]);
});

it("requires optional browser capabilities to stay out of global setup", () => {
  expect(
    auditUiSourceFile({
      relativePath: "testing/vitest.setup.ts",
      source: 'import "./vitest-dom-capabilities.setup";',
    }),
  ).toEqual([
    "testing/vitest.setup.ts: optional browser and editor capabilities must be installed by the tests that need them",
  ]);
});

it("keeps DashboardScreen renderers out of generic test helpers", () => {
  expect(
    auditUiSourceFile({
      relativePath: "testing/app-shell.tsx",
      source:
        'import { DashboardScreen } from "../features/dashboard/public/screen";',
    }),
  ).toEqual([
    "testing/app-shell.tsx: generic test helpers cannot import DashboardScreen; keep dashboard renderers feature-owned",
  ]);
});

it("rejects retired root App tests and App imports", () => {
  expect(
    auditUiTestFile({
      relativePath: "App.timeline.test.tsx",
      source: 'import { App } from "./App";',
    }),
  ).toHaveLength(2);
});

it("rejects DOM dependencies in unit tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.unit.test.ts",
      source: 'import { render } from "@testing-library/react";',
    }),
  ).toEqual([
    "features/example/lib/value.unit.test.ts: unit tests cannot import DOM or browser runners",
  ]);
});

it("rejects DOM dependencies in Bun unit tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.bun.unit.test.ts",
      source: 'import { render } from "@testing-library/react";',
    }),
  ).toEqual([
    "features/example/lib/value.bun.unit.test.ts: Bun unit tests cannot import DOM dependencies; move rendered behavior to a component test",
  ]);
});

it("rejects browser dependencies in Bun unit tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.bun.unit.test.ts",
      source: 'import { chromium } from "playwright";',
    }),
  ).toEqual([
    "features/example/lib/value.bun.unit.test.ts: Bun unit tests cannot import browser runners or integration harnesses; move browser behavior to an integration test",
  ]);
});

it("rejects an unowned native Bun unit import", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.test.ts",
      source: 'import { expect, it } from "bun:test";',
    }),
  ).toEqual([
    "features/example/lib/value.test.ts: files importing bun:test must use the exact .bun.unit.test.ts suffix so the Bun unit lane owns them",
  ]);
});

it("rejects DOM environment directives in legacy unit tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.test.ts",
      source: "// @vitest-environment jsdom",
    }),
  ).toEqual([
    "features/example/lib/value.test.ts: unit tests cannot request a DOM environment; rename the file as a component test",
  ]);
});

it("rejects optional DOM capability setup in unit tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/lib/value.test.ts",
      source: 'import "../../../testing/vitest-dom-capabilities.setup";',
    }),
  ).toEqual([
    "features/example/lib/value.test.ts: unit tests cannot install optional DOM capabilities",
  ]);
});

it("rejects browser runners in component tests", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/components/card.component.test.tsx",
      source: 'import { chromium } from "playwright";',
    }),
  ).toEqual([
    "features/example/components/card.component.test.tsx: component tests cannot import browser runners",
  ]);
});

it("accepts a focused component test", () => {
  expect(
    auditUiTestFile({
      relativePath: "features/example/components/card.component.test.tsx",
      source: 'import { render } from "@testing-library/react";',
    }),
  ).toEqual([]);
});
