import type { Stage } from './api';

/** Pipeline stages in display order (R11). */
export const STAGES: readonly { id: Stage; label: string }[] = [
  { id: 'lead', label: 'Lead' },
  { id: 'ready', label: 'Ready' },
  { id: 'applied', label: 'Applied' },
  { id: 'interviewing', label: 'Interviewing' },
  { id: 'offer', label: 'Offer' },
  { id: 'rejected', label: 'Rejected' },
  { id: 'withdrawn', label: 'Withdrawn' },
];

export function stageLabel(id: string): string {
  return STAGES.find((s) => s.id === id)?.label ?? id;
}
