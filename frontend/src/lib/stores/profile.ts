import { api } from '../api';
import { entityStore } from './entity';

/** The user's profile (R9). */
export const profileStore = entityStore(['profile'], () => api.getProfile());
