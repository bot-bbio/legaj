/**
 * Entity stores: the only place the UI holds backend data (C1, U1).
 *
 * A store loads its data when the first view subscribes, refetches when Go
 * reports a change to one of its entities, and stops listening when the
 * last view unsubscribes. Views render the store and never keep their own
 * copies, so every view showing an entity updates together (R3).
 */
import { writable, type Readable } from 'svelte/store';

import { errorMessage } from '../errors';
import { onEntityChanged, type Entity } from '../events';

export type Loadable<T> = { status: 'loading' } | { status: 'ready'; data: T } | { status: 'error'; message: string };

export interface EntityStore<T> extends Readable<Loadable<T>> {
  /** Refetch now (e.g. after a call whose result the view needs at once). */
  refresh(): Promise<void>;
}

/**
 * Bursts of changes (40 clips) are coalesced into one reload after
 * `debounceMs`, well inside R3's 500 ms budget.
 */
export function entityStore<T>(entities: readonly Entity[], load: () => Promise<T>, debounceMs = 100): EntityStore<T> {
  let latest = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const { subscribe, set } = writable<Loadable<T>>({ status: 'loading' }, () => {
    void refresh();
    const off = onEntityChanged((change) => {
      if (!entities.includes(change.entity)) return;
      clearTimeout(timer);
      timer = setTimeout(() => void refresh(), debounceMs);
    });
    return () => {
      clearTimeout(timer);
      off();
      // Forget the data: the next subscriber must never act on a stale copy.
      latest++;
      set({ status: 'loading' });
    };
  });

  async function refresh(): Promise<void> {
    const call = ++latest;
    try {
      const data = await load();
      // A slower, older load must never overwrite a newer one.
      if (call === latest) set({ status: 'ready', data });
    } catch (e) {
      if (call === latest) set({ status: 'error', message: errorMessage(e) });
    }
  }

  return { subscribe, refresh };
}
