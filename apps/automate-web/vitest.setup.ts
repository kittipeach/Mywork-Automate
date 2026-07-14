import '@testing-library/jest-dom/vitest';

// React Flow (and some UI) query ResizeObserver, which jsdom lacks.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
const g = globalThis as unknown as { ResizeObserver?: unknown };
g.ResizeObserver ??= ResizeObserverStub;
