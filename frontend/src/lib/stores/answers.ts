import { api } from '../api';
import { entityStore } from './entity';

/** Every banked answer, most recent first (R25). */
export const answerBankStore = entityStore(['answer'], () => api.answerBank());
