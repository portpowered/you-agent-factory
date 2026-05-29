/**
 * Bun unit/coverage lane: register jsdom globals before any test or Testing Library import.
 * Do not import @testing-library/react here — ES module evaluation order requires document
 * to exist before @testing-library/dom initializes screen queries.
 */
import { JSDOM } from "jsdom";

const dom = new JSDOM("<!DOCTYPE html><html><body></body></html>", {
  url: "http://localhost/",
});

const { window } = dom;

const globalScope = globalThis as typeof globalThis & {
  window: Window & typeof globalThis;
  document: Document;
  navigator: Navigator;
  HTMLElement: typeof HTMLElement;
  Element: typeof Element;
  Node: typeof Node;
  Text: typeof Text;
  DocumentFragment: typeof DocumentFragment;
  Event: typeof Event;
  CustomEvent: typeof CustomEvent;
  MouseEvent: typeof MouseEvent;
  KeyboardEvent: typeof KeyboardEvent;
  FocusEvent: typeof FocusEvent;
  InputEvent: typeof InputEvent;
  getComputedStyle: typeof window.getComputedStyle;
  requestAnimationFrame?: typeof window.requestAnimationFrame;
  cancelAnimationFrame?: typeof window.cancelAnimationFrame;
};

globalScope.window = window as Window & typeof globalThis;
globalScope.document = window.document;
globalScope.navigator = window.navigator;
globalScope.HTMLElement = window.HTMLElement;
globalScope.Element = window.Element;
globalScope.Node = window.Node;
globalScope.Text = window.Text;
globalScope.DocumentFragment = window.DocumentFragment;
globalScope.Event = window.Event;
globalScope.CustomEvent = window.CustomEvent;
globalScope.MouseEvent = window.MouseEvent;
globalScope.KeyboardEvent = window.KeyboardEvent;
globalScope.FocusEvent = window.FocusEvent;
globalScope.InputEvent = window.InputEvent;
globalScope.getComputedStyle = window.getComputedStyle.bind(window);

if (typeof window.requestAnimationFrame === "function") {
  globalScope.requestAnimationFrame = window.requestAnimationFrame.bind(window);
  globalScope.cancelAnimationFrame = window.cancelAnimationFrame.bind(window);
} else {
  globalScope.requestAnimationFrame = (callback: FrameRequestCallback) =>
    setTimeout(() => callback(Date.now()), 0) as unknown as number;
  globalScope.cancelAnimationFrame = (handle: number) => {
    clearTimeout(handle);
  };
}
