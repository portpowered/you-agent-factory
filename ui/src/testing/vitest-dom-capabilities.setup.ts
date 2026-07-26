import {
  installLocalStorageShim,
  installResizeObserverShim,
} from "../components/dashboard/test-browser-shims";

installResizeObserverShim();
installLocalStorageShim();

if (typeof HTMLAnchorElement !== "undefined") {
  const originalAnchorClick = HTMLAnchorElement.prototype.click;

  HTMLAnchorElement.prototype.click = function click(): void {
    if (this.download.length > 0) {
      return;
    }

    originalAnchorClick.call(this);
  };
}

if (typeof document !== "undefined" && !document.queryCommandSupported) {
  document.queryCommandSupported = () => false;
}
