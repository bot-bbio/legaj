/** The app's top-level sections (R1). The order here is the order in the nav. */
export type SectionId = 'profile' | 'pipeline' | 'followups' | 'interviews' | 'settings';

export interface NavSection {
  readonly id: SectionId;
  readonly label: string;
}

/**
 * Exactly five sections. Review queue and Answer bank live inside Pipeline
 * (R43), so they never become nav entries.
 */
export const NAV_SECTIONS: readonly NavSection[] = [
  { id: 'profile', label: 'Profile' },
  { id: 'pipeline', label: 'Pipeline' },
  { id: 'followups', label: 'Follow-ups' },
  { id: 'interviews', label: 'Interviews' },
  { id: 'settings', label: 'Settings' },
];

/** Profile is where the user lands after setup (U8). */
export const DEFAULT_SECTION: SectionId = 'profile';

export function sectionLabel(id: SectionId): string {
  const section = NAV_SECTIONS.find((s) => s.id === id);
  return section ? section.label : id;
}
