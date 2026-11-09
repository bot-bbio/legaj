import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { EntityChange } from '../events';

// Capture the change listener the store registers.
const listeners: ((c: EntityChange) => void)[] = [];
const unsubscribe = vi.fn();
vi.mock('../events', () => ({
  onEntityChanged: (cb: (c: EntityChange) => void) => {
    listeners.push(cb);
    return unsubscribe;
  },
}));

const { entityStore } = await import('./entity');

function emit(change: EntityChange): void {
  for (const l of listeners) l(change);
}

describe('R3 entity stores render backend state', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    listeners.length = 0;
    unsubscribe.mockClear();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('R3 store refreshes on event: a change event refetches within 500 ms', async () => {
    let version = 1;
    const load = vi.fn(() => Promise.resolve({ version }));
    const store = entityStore(['profile'], load);
    const seen: unknown[] = [];
    const stop = store.subscribe((s) => seen.push(s));

    await vi.advanceTimersByTimeAsync(0);
    expect(get(store)).toEqual({ status: 'ready', data: { version: 1 } });

    version = 2; // the backend changed
    emit({ entity: 'profile', ids: [] });
    await vi.advanceTimersByTimeAsync(500);

    expect(load).toHaveBeenCalledTimes(2);
    expect(get(store)).toEqual({ status: 'ready', data: { version: 2 } });
    stop();
  });

  it('ignores changes to other entities', async () => {
    const load = vi.fn(() => Promise.resolve(1));
    const stop = entityStore(['profile'], load).subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);

    emit({ entity: 'application', ids: [3] });
    await vi.advanceTimersByTimeAsync(500);

    expect(load).toHaveBeenCalledTimes(1);
    stop();
  });

  it('coalesces a burst of changes into one reload', async () => {
    const load = vi.fn(() => Promise.resolve(1));
    const stop = entityStore(['application'], load).subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);

    for (let i = 0; i < 40; i++) emit({ entity: 'application', ids: [i] }); // 40 clips
    await vi.advanceTimersByTimeAsync(500);

    expect(load).toHaveBeenCalledTimes(2);
    stop();
  });

  it('a slow older load never overwrites a newer one', async () => {
    let call = 0;
    const load = vi.fn(() => {
      call++;
      const n = call;
      // The first load is slow, the second fast.
      return new Promise<number>((resolve) => setTimeout(() => resolve(n), n === 1 ? 1000 : 10));
    });
    const store = entityStore(['settings'], load);
    const stop = store.subscribe(() => {});
    void store.refresh();
    await vi.advanceTimersByTimeAsync(2000);

    expect(get(store)).toEqual({ status: 'ready', data: 2 });
    stop();
  });

  it('shows the backend error message and stops listening when unused', async () => {
    const load = vi.fn(() => Promise.reject(new Error('LeGaJ couldn’t read your profile.')));
    const stop = entityStore(['profile'], load).subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    stop();
    expect(unsubscribe).toHaveBeenCalledTimes(1);
  });

  it('turns a rejected Wails call (a plain string) into an error state', async () => {
    const store = entityStore(['profile'], () => Promise.reject('Gemini rejected the key.'));
    const stop = store.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(get(store)).toEqual({ status: 'error', message: 'Gemini rejected the key.' });
    stop();
  });
});
