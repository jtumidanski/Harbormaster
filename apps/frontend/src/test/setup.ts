import "@testing-library/jest-dom/vitest";

// jsdom does not implement matchMedia; ThemeProvider needs it.
if (typeof window.matchMedia !== "function") {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    configurable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// jsdom's localStorage is not a real Storage implementation in this setup;
// install a minimal in-memory shim so consumers of window.localStorage work.
function installStorageShim(key: "localStorage" | "sessionStorage") {
  const existing = (window as unknown as Record<string, unknown>)[key] as
    | { getItem?: unknown }
    | undefined;
  if (existing && typeof existing.getItem === "function") return;
  const store = new Map<string, string>();
  const shim: Storage = {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(k: string) {
      return store.has(k) ? (store.get(k) as string) : null;
    },
    key(i: number) {
      return Array.from(store.keys())[i] ?? null;
    },
    removeItem(k: string) {
      store.delete(k);
    },
    setItem(k: string, v: string) {
      store.set(k, String(v));
    },
  };
  Object.defineProperty(window, key, {
    writable: true,
    configurable: true,
    value: shim,
  });
}
installStorageShim("localStorage");
installStorageShim("sessionStorage");

// jsdom does not implement ResizeObserver which Recharts' ResponsiveContainer
// relies on. Install a no-op shim so chart tests don't throw.
if (typeof window.ResizeObserver === "undefined") {
  window.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// jsdom does not implement PointerEvent / pointer capture APIs that Radix
// primitives (Select, Dialog) call into during interaction.
type ElProto = HTMLElement & {
  hasPointerCapture?: (id: number) => boolean;
  setPointerCapture?: (id: number) => void;
  releasePointerCapture?: (id: number) => void;
  scrollIntoView?: () => void;
};
const proto = HTMLElement.prototype as ElProto;
if (typeof proto.hasPointerCapture !== "function") {
  proto.hasPointerCapture = () => false;
}
if (typeof proto.setPointerCapture !== "function") {
  proto.setPointerCapture = () => {};
}
if (typeof proto.releasePointerCapture !== "function") {
  proto.releasePointerCapture = () => {};
}
if (typeof proto.scrollIntoView !== "function") {
  proto.scrollIntoView = () => {};
}
