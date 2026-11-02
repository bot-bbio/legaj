import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

// The views behind the nav load their own data; give them empty answers.
vi.mock('./lib/events', () => ({ onEntityChanged: () => () => {} }));
vi.mock('./lib/api', () => ({
  api: {
    startup: () => Promise.resolve({ kind: 'ready', movedAside: '', backups: [], message: '' }),
    onboardingState: () =>
      Promise.resolve({
        finished: true,
        steps: ['key', 'resume', 'profile', 'clipper'].map((id) => ({ id, done: true })),
      }),
    getProfile: () =>
      Promise.resolve({
        fullName: '', email: '', phone: '', location: '', headline: '', targetRole: '', summary: '',
        links: [], experiences: [], education: [], skills: [],
      }),
    getSettings: () =>
      Promise.resolve({
        settings: { model: 'm', skipReview: false, defaultMaterials: 'letter', followUpDays: 7, updateCheck: true, startWithWindows: false },
        key: { saved: false, masked: '' },
      }),
  },
}));

const { default: App } = await import('./App.svelte');

async function mainNav(): Promise<HTMLElement> {
  render(App);
  return screen.findByRole('navigation', { name: 'Main' });
}

describe('R1 navigation', () => {
  it('TestR1_NavOrder: shows exactly the five sections, in order', async () => {
    const nav = await mainNav();
    const labels = within(nav)
      .getAllByRole('button')
      .map((button) => button.textContent.trim());
    // Hard-coded on purpose: the test must not import the list it checks.
    expect(labels).toEqual(['Profile', 'Pipeline', 'Follow-ups', 'Interviews', 'Settings']);
  });

  it('TestR1_ProfileIsDefault: Profile is the current section on start (U8)', async () => {
    const nav = await mainNav();
    const current = within(nav).getByRole('button', { current: 'page' });
    expect(current).toHaveTextContent('Profile');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Profile');
  });

  it('switches the view when a section is chosen', async () => {
    const nav = await mainNav();
    await userEvent.click(within(nav).getByRole('button', { name: 'Interviews' }));
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Interviews');
    expect(within(nav).getByRole('button', { current: 'page' })).toHaveTextContent('Interviews');
  });
});
