import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  mock,
  test,
} from "bun:test";
import * as domMatchers from "@testing-library/jest-dom/matchers";
import { Window } from "happy-dom";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import * as applicationReact from "react";
import * as monacoEditorAll from "../mocks/monaco-editor-all";
import * as monacoEditorApi from "../mocks/monaco-editor-api";
import * as monacoReact from "../mocks/monaco-react";
import { bunVi } from "./vi-compat";

mock.module("@monaco-editor/react", () => monacoReact);
mock.module("monaco-editor/esm/vs/editor/editor.all.js", () => monacoEditorAll);
mock.module("monaco-editor/esm/vs/editor/editor.api.js", () => monacoEditorApi);

const REACT_INTERNALS_KEY =
  "__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE";

type ReactModuleWithInternals = typeof applicationReact & {
  [REACT_INTERNALS_KEY]: Record<PropertyKey, unknown>;
};

async function shareApplicationReactDispatcher(packageReactPath: string) {
  if (!existsSync(packageReactPath)) {
    return;
  }

  const packageReact = (await import(
    pathToFileURL(packageReactPath).href
  )) as ReactModuleWithInternals;
  const applicationInternals = (
    applicationReact as ReactModuleWithInternals
  )[REACT_INTERNALS_KEY];
  const packageInternals = packageReact[REACT_INTERNALS_KEY];

  for (const key of Reflect.ownKeys(applicationInternals)) {
    const descriptor = Object.getOwnPropertyDescriptor(
      applicationInternals,
      key,
    );
    Object.defineProperty(packageInternals, key, {
      configurable: true,
      enumerable: descriptor?.enumerable ?? true,
      get: () => applicationInternals[key],
      set: (value) => {
        applicationInternals[key] = value;
      },
    });
  }
}

for (const packageDirectory of ["components"]) {
  await shareApplicationReactDispatcher(
    resolve(
      import.meta.dir,
      `../../../packages/${packageDirectory}/node_modules/react/index.js`,
    ),
  );
}

const bunTestWindow = new Window({ url: "http://localhost/" });

Object.assign(bunTestWindow, {
  Error,
  RangeError,
  ReferenceError,
  SyntaxError,
  TypeError,
});

for (const key of Object.getOwnPropertyNames(bunTestWindow)) {
  if (!(key in globalThis)) {
    Object.defineProperty(
      globalThis,
      key,
      Object.getOwnPropertyDescriptor(bunTestWindow, key)!,
    );
  }
}

Object.defineProperties(globalThis, {
  document: { configurable: true, value: bunTestWindow.document },
  navigator: { configurable: true, value: bunTestWindow.navigator },
  window: { configurable: true, value: bunTestWindow },
});

for (const key of [
  "CustomEvent",
  "Event",
  "FocusEvent",
  "InputEvent",
  "KeyboardEvent",
  "MouseEvent",
  "PointerEvent",
  "UIEvent",
] as const) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: bunTestWindow[key],
  });
}

// happy-dom's Window is separate from Bun's global object. Keep browser APIs
// that tests intentionally shim synchronized across both surfaces, as they are
// when Vitest runs under jsdom.
for (const key of [
  "cancelAnimationFrame",
  "DOMMatrixReadOnly",
  "EventSource",
  "requestAnimationFrame",
  "ResizeObserver",
] as const) {
  Object.defineProperty(bunTestWindow, key, {
    configurable: true,
    get: () => globalThis[key],
    set: (value) => {
      globalThis[key] = value as never;
    },
  });
}

expect.extend(domMatchers);

const { cleanup, configure } = await import("@testing-library/react");

Object.assign(globalThis, {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  IS_REACT_ACT_ENVIRONMENT: true,
  it,
  test,
  vi: bunVi,
});

configure({
  asyncUtilTimeout: 10_000,
});

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
  document.body.removeAttribute("style");
  document.documentElement.removeAttribute("data-color-palette");
  document.documentElement.removeAttribute("style");
});
