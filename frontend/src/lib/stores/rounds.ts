import { api } from '../api';
import { entityStore } from './entity';

/** Upcoming and Past interview rounds (R30). */
export const roundsStore = entityStore(['round', 'application'], () => api.interviewLists());
