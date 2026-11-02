import { writable } from 'svelte/store';

import { api, type StepID } from '../api';
import { entityStore } from './entity';

/**
 * Setup progress (R7). Steps complete as their data appears, so the store
 * listens to every entity a step depends on.
 */
export const onboardingStore = entityStore(['settings', 'resume', 'profile', 'application'], () => api.onboardingState());

const STEPS: readonly string[] = ['key', 'resume', 'profile', 'clipper'];

export function isStepID(id: string): id is StepID {
  return STEPS.includes(id);
}

/** Whether the wizard is showing, and at which step. */
export const wizard = writable<{ open: boolean; step: StepID }>({ open: false, step: 'key' });

export function openWizard(step: StepID = 'key'): void {
  wizard.set({ open: true, step });
}

export function closeWizard(): void {
  wizard.set({ open: false, step: 'key' });
}
