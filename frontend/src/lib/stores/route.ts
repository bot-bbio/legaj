import { writable } from 'svelte/store';

import { DEFAULT_SECTION, type SectionId } from '../nav';

/** The visible top-level section. Any component may navigate (e.g. onboarding's Finish). */
export const route = writable<SectionId>(DEFAULT_SECTION);

export function navigate(section: SectionId): void {
  route.set(section);
}
