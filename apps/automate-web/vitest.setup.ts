import '@testing-library/jest-dom/vitest';

// React Flow (and some UI) query ResizeObserver, which jsdom lacks.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
// @ts-expect-error jsdom global
global.ResizeObserver = global.ResizeObserver ?? ResizeObserverStub;
