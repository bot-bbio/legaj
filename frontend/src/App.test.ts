import { render, screen, within } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';

import App from './App.svelte';

describe('R1 navigation', () => {
  it('TestR1_NavOrder: shows exactly the five sections, in order', () => {
    render(App);
    const nav = screen.getByRole('navigation', { name: 'Main' });
    const labels = within(nav)
      .getAllByRole('button')
      .map((button) => button.textContent.trim());
    // Hard-coded on purpose: the test must not import the list it checks.
    expect(labels).toEqual(['Profile', 'Pipeline', 'Follow-ups', 'Interviews', 'Settings']);
  });

  it('TestR1_ProfileIsDefault: Profile is the current section on start (U8)', () => {
    render(App);
    const nav = screen.getByRole('navigation', { name: 'Main' });
    const current = within(nav).getByRole('button', { current: 'page' });
    expect(current).toHaveTextContent('Profile');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Profile');
  });
});
