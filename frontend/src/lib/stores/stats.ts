import { api } from '../api';
import { entityStore } from './entity';

/** The Pipeline header: counts, newest 5, next interview (R14). */
export const statsStore = entityStore(['application', 'round'], () => api.pipelineStats());
