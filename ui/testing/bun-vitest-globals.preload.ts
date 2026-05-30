/**
 * Merge Vitest `vi` helpers onto Bun's runner `vi` so specs keep stubGlobal/hoisted/mocked
 * after Bun registers its own test globals.
 */
import { vi as vitestVi } from "vitest";

const stubbedGlobals = new Map<string, unknown>();
const stubbedEnvs = new Map<string, string | undefined>();

function stubEnv(key: string, value: string): void {
  if (!stubbedEnvs.has(key)) {
    stubbedEnvs.set(key, process.env[key]);
  }
  process.env[key] = value;
}

function unstubAllEnvs(): void {
  for (const [key, original] of stubbedEnvs) {
    if (original === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = original;
    }
  }
  stubbedEnvs.clear();
}

const runnerVi = (globalThis as typeof globalThis & { vi?: typeof vitestVi }).vi ?? vitestVi;

function stubGlobal(name: string, value: unknown): void {
  if (!stubbedGlobals.has(name)) {
    stubbedGlobals.set(name, (globalThis as Record<string, unknown>)[name]);
  }
  assignGlobal(name, value);
}

function assignGlobal(name: string, value: unknown): void {
  try {
    (globalThis as Record<string, unknown>)[name] = value;
  } catch {
    // Some jsdom globals are read-only on globalThis.
  }

  if (typeof globalThis.window !== "undefined") {
    try {
      (globalThis.window as Record<string, unknown>)[name] = value;
    } catch {
      // Match Vitest behavior: ignore read-only window properties on restore.
    }
  }
}

function unstubAllGlobals(): void {
  for (const [name, original] of stubbedGlobals) {
    if (original === undefined) {
      try {
        delete (globalThis as Record<string, unknown>)[name];
      } catch {
        // ignore
      }
      if (typeof globalThis.window !== "undefined") {
        try {
          delete (globalThis.window as Record<string, unknown>)[name];
        } catch {
          // ignore
        }
      }
      continue;
    }

    assignGlobal(name, original);
  }
  stubbedGlobals.clear();
}

Object.assign(runnerVi, {
  stubGlobal:
    typeof vitestVi.stubGlobal === "function"
      ? vitestVi.stubGlobal.bind(vitestVi)
      : stubGlobal,
  unstubAllGlobals:
    typeof vitestVi.unstubAllGlobals === "function"
      ? vitestVi.unstubAllGlobals.bind(vitestVi)
      : unstubAllGlobals,
  hoisted:
    typeof vitestVi.hoisted === "function"
      ? vitestVi.hoisted.bind(vitestVi)
      : <T>(factory: () => T) => factory(),
  importActual:
    typeof vitestVi.importActual === "function"
      ? vitestVi.importActual.bind(vitestVi)
      : async (path: string) => import(path),
  importMock:
    typeof vitestVi.importMock === "function"
      ? vitestVi.importMock.bind(vitestVi)
      : async (path: string) => import(path),
  mocked:
    typeof vitestVi.mocked === "function"
      ? vitestVi.mocked.bind(vitestVi)
      : <T>(item: T) => item,
  stubEnv:
    typeof vitestVi.stubEnv === "function"
      ? vitestVi.stubEnv.bind(vitestVi)
      : stubEnv,
  unstubAllEnvs:
    typeof vitestVi.unstubAllEnvs === "function"
      ? vitestVi.unstubAllEnvs.bind(vitestVi)
      : unstubAllEnvs,
});

(globalThis as typeof globalThis & { vi: typeof runnerVi }).vi = runnerVi;
