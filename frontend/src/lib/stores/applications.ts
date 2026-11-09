import { api } from '../api';
import { entityStore } from './entity';

/** Every live application, newest first (R12). */
export const applicationsStore = entityStore(['application'], () => api.listApplications());
