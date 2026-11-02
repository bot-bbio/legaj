import { api } from '../api';
import { entityStore } from './entity';

/**
 * The Review panel (R23): generation jobs and drafts awaiting review. Jobs
 * report progress as `job` changes; approving a draft is a `document`
 * change; deleting a row is an `application` change.
 */
export const reviewStore = entityStore(['job', 'document', 'application'], () => api.reviewQueue());
