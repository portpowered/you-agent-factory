/// <reference types="vite/client" />

declare function setImmediate(
  handler: (...args: unknown[]) => void,
  ...args: unknown[]
): number;

declare function clearImmediate(handle: number): void;

