import type { Profile } from './api';

const pick = (draft: string, saved: string): string => (draft.trim() !== '' ? draft : saved);

/**
 * Combines a freshly parsed resume (draft) with the saved profile, so
 * reading a new resume never silently erases what the user typed:
 * - contact details and summary come from the draft only where it has them;
 * - the target role is the user's own words and is kept (U1);
 * - links are merged by URL;
 * - experience, education and skills come from the draft when it has any
 *   (the resume is their source), otherwise the saved ones stay.
 */
export function mergeDraft(saved: Profile, draft: Profile): Profile {
  const urls = new Set(saved.links.map((l) => l.url));
  return {
    fullName: pick(draft.fullName, saved.fullName),
    email: pick(draft.email, saved.email),
    phone: pick(draft.phone, saved.phone),
    location: pick(draft.location, saved.location),
    headline: pick(draft.headline, saved.headline),
    summary: pick(draft.summary, saved.summary),
    targetRole: pick(saved.targetRole, draft.targetRole),
    links: [...saved.links, ...draft.links.filter((l) => !urls.has(l.url))],
    experiences: draft.experiences.length > 0 ? draft.experiences : saved.experiences,
    education: draft.education.length > 0 ? draft.education : saved.education,
    skills: draft.skills.length > 0 ? draft.skills : saved.skills,
  };
}

/** True when saving the merge would replace experience, education or skills already saved. */
export function replacesHistory(saved: Profile, draft: Profile): boolean {
  return (
    (draft.experiences.length > 0 && saved.experiences.length > 0) ||
    (draft.education.length > 0 && saved.education.length > 0) ||
    (draft.skills.length > 0 && saved.skills.length > 0)
  );
}
