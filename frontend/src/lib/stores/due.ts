import { api } from '../api';
import { entityStore } from './entity';

/**
 * Follow-ups due now (R28). Due-ness also changes with time alone (a round
 * ends, a new day starts), so the nav refreshes this store every minute.
 */
export const dueStore = entityStore(['application', 'followup', 'round', 'settings'], () => api.followupsDue());

export const DUE_REFRESH_MS = 60_000;
