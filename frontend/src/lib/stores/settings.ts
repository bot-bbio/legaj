import { api } from '../api';
import { entityStore } from './entity';

/** Settings and the key's masked status (R37, R4). */
export const settingsStore = entityStore(['settings'], () => api.getSettings());
