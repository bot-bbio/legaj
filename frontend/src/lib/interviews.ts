/** Interview round helpers shared by the Interviews view and its editor. */

export const KINDS: readonly { id: string; label: string }[] = [
  { id: 'phone', label: 'Phone' },
  { id: 'video', label: 'Video' },
  { id: 'onsite', label: 'On site' },
  { id: 'technical', label: 'Technical' },
  { id: 'panel', label: 'Panel' },
  { id: 'other', label: 'Other' },
];

export function kindLabel(id: string): string {
  return KINDS.find((k) => k.id === id)?.label ?? 'Other';
}

/** The user's own IANA zone, as Windows reports it to the WebView. */
export function userZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}

/** Every IANA zone the WebView knows, plus UTC, the user's and any extra. */
export function zones(extra?: string): string[] {
  const set = new Set<string>(Intl.supportedValuesOf('timeZone'));
  set.add('UTC');
  set.add(userZone());
  if (extra) set.add(extra);
  return [...set].sort();
}

/** "2026-11-03T18:00" as "3 Nov 2026, 18:00", without converting zones. */
export function wallClock(local: string): string {
  const [date, time] = local.split('T');
  if (!date || !time) return local;
  const [y, m, d] = date.split('-').map(Number);
  if (!y || !m || !d) return local;
  const day = new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(undefined, {
    dateStyle: 'medium',
    timeZone: 'UTC',
  });
  return `${day}, ${time}`;
}
