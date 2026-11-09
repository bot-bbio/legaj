import { describe, expect, it, vi } from 'vitest';

const handlers = new Map<string, (...data: unknown[]) => void>();
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, cb: (...data: unknown[]) => void) => {
    handlers.set(name, cb);
    return () => handlers.delete(name);
  },
}));

const { onEntityChanged, CHANGED_EVENT } = await import('./events');

describe('R3 change events', () => {
  it('passes well-formed entity:changed payloads through', () => {
    const seen: unknown[] = [];
    const off = onEntityChanged((c) => seen.push(c));
    handlers.get(CHANGED_EVENT)?.({ entity: 'application', ids: [1, 2] });
    expect(seen).toEqual([{ entity: 'application', ids: [1, 2] }]);
    off();
    expect(handlers.has(CHANGED_EVENT)).toBe(false);
  });

  it('drops malformed payloads', () => {
    const seen: unknown[] = [];
    onEntityChanged((c) => seen.push(c));
    const emit = handlers.get(CHANGED_EVENT);
    emit?.(null);
    emit?.({ entity: 'nonsense', ids: [] });
    emit?.({ entity: 'profile', ids: ['x'] });
    emit?.('profile');
    expect(seen).toEqual([]);
  });
});
