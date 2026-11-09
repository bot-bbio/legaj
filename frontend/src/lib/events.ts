/**
 * Backend change notifications (R3). Go emits `entity:changed` after every
 * committed write (internal/events). Entity stores listen and refetch.
 */
import { EventsOn } from '../../wailsjs/runtime/runtime';

export const CHANGED_EVENT = 'entity:changed';

export const ENTITIES = ['application', 'profile', 'settings', 'resume', 'document', 'answer', 'round', 'followup', 'job'] as const;
export type Entity = (typeof ENTITIES)[number];

export interface EntityChange {
  entity: Entity;
  ids: number[];
}

function isEntityChange(p: unknown): p is EntityChange {
  if (typeof p !== 'object' || p === null) return false;
  const { entity, ids } = p as { entity?: unknown; ids?: unknown };
  return (
    typeof entity === 'string' &&
    (ENTITIES as readonly string[]).includes(entity) &&
    Array.isArray(ids) &&
    ids.every((id) => typeof id === 'number')
  );
}

/** Calls cb for each well-formed change event. Returns the unsubscribe function. */
export function onEntityChanged(cb: (change: EntityChange) => void): () => void {
  return EventsOn(CHANGED_EVENT, (payload: unknown) => {
    if (isEntityChange(payload)) cb(payload);
  });
}
