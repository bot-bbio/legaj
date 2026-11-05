import { api } from '../api';
import { entityStore } from './entity';

/** Where the clipper listens, and whether the bookmarklet needs reinstalling (R18). */
export const clipStore = entityStore(['settings'], () => api.clipStatus());
