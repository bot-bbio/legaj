import { api } from '../api';
import { entityStore } from './entity';

/** The resume new applications will use; id 0 means none yet (R10). */
export const resumeStore = entityStore(['resume'], () => api.currentResume());
