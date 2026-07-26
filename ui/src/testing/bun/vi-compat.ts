import { type Mock, mock, spyOn } from "bun:test";

const stubbedGlobals = new Map<PropertyKey, PropertyDescriptor | undefined>();
const stubbedWindowGlobals = new Map<
  PropertyKey,
  PropertyDescriptor | undefined
>();

function secondaryWindowGlobal(): object | null {
  const candidate = (globalThis as typeof globalThis & { window?: object })
    .window;
  return candidate && candidate !== globalThis ? candidate : null;
}

function restoreDescriptor(
  target: object,
  key: PropertyKey,
  descriptor: PropertyDescriptor | undefined,
) {
  if (descriptor) {
    Object.defineProperty(target, key, descriptor);
  } else {
    Reflect.deleteProperty(target, key);
  }
}

export const bunVi = {
  clearAllMocks: mock.clearAllMocks,
  fn: mock,
  mocked<T extends (...args: never[]) => unknown>(value: T): Mock<T> {
    return value as unknown as Mock<T>;
  },
  spyOn,
  restoreAllMocks: mock.restore,
  stubGlobal(key: PropertyKey, value: unknown) {
    if (!stubbedGlobals.has(key)) {
      stubbedGlobals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    }
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
    const testWindow = secondaryWindowGlobal();
    if (testWindow) {
      if (!stubbedWindowGlobals.has(key)) {
        stubbedWindowGlobals.set(
          key,
          Object.getOwnPropertyDescriptor(testWindow, key),
        );
      }
      Object.defineProperty(testWindow, key, {
        configurable: true,
        writable: true,
        value,
      });
    }
  },
  unstubAllGlobals() {
    for (const [key, descriptor] of stubbedGlobals) {
      restoreDescriptor(globalThis, key, descriptor);
    }
    const testWindow = secondaryWindowGlobal();
    if (testWindow) {
      for (const [key, descriptor] of stubbedWindowGlobals) {
        restoreDescriptor(testWindow, key, descriptor);
      }
    }
    stubbedGlobals.clear();
    stubbedWindowGlobals.clear();
  },
};
