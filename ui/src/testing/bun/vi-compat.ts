import { mock, spyOn, type Mock } from "bun:test";

const stubbedGlobals = new Map<PropertyKey, PropertyDescriptor | undefined>();

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
      stubbedGlobals.set(
        key,
        Object.getOwnPropertyDescriptor(globalThis, key),
      );
    }
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
  },
  unstubAllGlobals() {
    for (const [key, descriptor] of stubbedGlobals) {
      if (descriptor) {
        Object.defineProperty(globalThis, key, descriptor);
      } else {
        Reflect.deleteProperty(globalThis, key);
      }
    }
    stubbedGlobals.clear();
  },
};
