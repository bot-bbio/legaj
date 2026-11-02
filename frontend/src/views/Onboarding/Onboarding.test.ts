import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { OnboardingState, Profile, ResumeImport } from '../../lib/api';

vi.mock('../../lib/events', () => ({ onEntityChanged: () => () => {} }));

const emptyProfile = (): Profile => ({
  fullName: '', email: '', phone: '', location: '', headline: '', targetRole: '', summary: '',
  links: [], experiences: [], education: [], skills: [],
});
const parsed = (): Profile => ({
  ...emptyProfile(),
  fullName: 'Ada Lovelace',
  email: 'ada@example.com',
  experiences: [
    { id: '', company: 'Babbage & Co', title: 'Analyst', location: '', start: { year: 1842, month: 1 }, end: { year: 1843, month: 9 }, current: false, bullets: [] },
  ],
});
const fresh = (): OnboardingState => ({
  finished: false,
  steps: [
    { id: 'key', done: false },
    { id: 'resume', done: false },
    { id: 'profile', done: false },
    { id: 'clipper', done: false },
  ],
});

let savedProfile: Profile = emptyProfile();
const api = {
  startup: vi.fn(() => Promise.resolve({ kind: 'ready', movedAside: '', backups: [], message: '' })),
  onboardingState: vi.fn<() => Promise<OnboardingState>>(),
  saveKeyAndTest: vi.fn(() => Promise.resolve({ verified: true, message: 'Key works.' })),
  uploadResume: vi.fn<() => Promise<ResumeImport>>(),
  parseResume: vi.fn<(id: number) => Promise<ResumeImport>>(),
  finishOnboarding: vi.fn(() => Promise.resolve()),
  getProfile: vi.fn(() => Promise.resolve(savedProfile)),
  saveProfile: vi.fn((p: Profile) => {
    savedProfile = p;
    return Promise.resolve(p);
  }),
  getSettings: vi.fn(),
};
vi.mock('../../lib/api', () => ({ api }));

const { default: App } = await import('../../App.svelte');
const { closeWizard } = await import('../../lib/stores/onboarding');

describe('R7 onboarding', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    closeWizard(); // UI state outlives a component; reset it between tests
    savedProfile = emptyProfile();
    api.onboardingState.mockResolvedValue(fresh());
  });

  it('opens on first run and shows the free-tier data-use disclosure', async () => {
    render(App);
    expect(await screen.findByRole('heading', { name: /set up legaj/i })).toBeInTheDocument();
    const notice = screen.getByText(/how your data is used/i).closest('aside');
    expect(notice).toHaveTextContent(/free tier/i);
    expect(notice).toHaveTextContent(/human reviewers may read it/i);
    expect(notice).toHaveTextContent(/paid tier/i);
    expect(screen.queryByRole('navigation', { name: 'Main' })).not.toBeInTheDocument();
  });

  it('key input has autocomplete off and is cleared after submit', async () => {
    render(App);
    const input = await screen.findByLabelText(/gemini api key/i);
    expect(input).toHaveAttribute('autocomplete', 'off');
    await userEvent.type(input, 'AIzaSyD-SENTINEL-KEY-0123456789abcd');
    await userEvent.click(screen.getByRole('button', { name: /save and test/i }));
    expect(api.saveKeyAndTest).toHaveBeenCalledWith('AIzaSyD-SENTINEL-KEY-0123456789abcd');
    expect(input).toHaveValue('');
    expect(await screen.findByText('Key works.')).toBeInTheDocument();
  });

  it('every step can be skipped, leaving a resumable checklist', async () => {
    render(App);
    await userEvent.click(await screen.findByRole('button', { name: /skip for now/i })); // key
    await userEvent.click(screen.getByRole('button', { name: /skip for now/i })); // resume
    await userEvent.click(await screen.findByRole('button', { name: /skip for now/i })); // profile
    await userEvent.click(screen.getByRole('button', { name: /finish/i }));

    expect(api.finishOnboarding).toHaveBeenCalledTimes(1);
    const checklist = await screen.findByRole('complementary', { name: /finish setting up/i });
    const items = within(checklist).getAllByRole('button').map((b) => b.textContent.trim());
    expect(items).toEqual(['Add your Gemini API key', 'Upload your resume', 'Check your profile', 'Install the clipper']);

    // Resuming opens the wizard at that step.
    await userEvent.click(within(checklist).getByRole('button', { name: /upload your resume/i }));
    expect(await screen.findByRole('button', { name: /choose resume file/i })).toBeInTheDocument();
  });

  it('Finish lands on a filled Profile without a reload, with the target role as entered (U1)', async () => {
    const reload = vi.fn();
    Object.defineProperty(window, 'location', { value: { ...window.location, reload }, configurable: true });
    api.uploadResume.mockResolvedValue({
      cancelled: false,
      file: { id: 1, fileName: 'cv.pdf', sizeBytes: 10, uploadedAt: '2026-11-01T09:00:00Z' },
      draft: parsed(),
      parseError: '',
    });

    render(App);
    await userEvent.click(await screen.findByRole('button', { name: /skip for now/i })); // key
    await userEvent.click(screen.getByRole('button', { name: /choose resume file/i }));

    const role = await screen.findByLabelText(/target role/i);
    await userEvent.type(role, 'Junior Data Engineer (remote)');
    expect(screen.getByDisplayValue('Ada Lovelace')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /save profile/i }));

    expect(api.saveProfile).toHaveBeenCalledWith(expect.objectContaining({ fullName: 'Ada Lovelace', targetRole: 'Junior Data Engineer (remote)' }));

    await userEvent.click(await screen.findByRole('button', { name: /finish/i }));
    const nav = await screen.findByRole('navigation', { name: 'Main' });
    expect(within(nav).getByRole('button', { current: 'page' })).toHaveTextContent('Profile');
    expect(await screen.findByDisplayValue('Ada Lovelace')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Junior Data Engineer (remote)')).toBeInTheDocument();
    expect(reload).not.toHaveBeenCalled();
  });

  it('a failed parse offers retry and manual entry', async () => {
    api.uploadResume.mockResolvedValue({
      cancelled: false,
      file: { id: 7, fileName: 'cv.pdf', sizeBytes: 10, uploadedAt: '2026-11-01T09:00:00Z' },
      parseError: 'Gemini’s reading of your resume didn’t pass LeGaJ’s checks.',
    });
    api.parseResume.mockResolvedValue({
      cancelled: false,
      file: { id: 7, fileName: 'cv.pdf', sizeBytes: 10, uploadedAt: '2026-11-01T09:00:00Z' },
      draft: parsed(),
      parseError: '',
    });
    render(App);
    await userEvent.click(await screen.findByRole('button', { name: /skip for now/i }));
    await userEvent.click(screen.getByRole('button', { name: /choose resume file/i }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/didn.t pass/i);

    await userEvent.click(screen.getByRole('button', { name: /try reading it again/i }));
    expect(api.parseResume).toHaveBeenCalledWith(7);
    expect(await screen.findByDisplayValue('Ada Lovelace')).toBeInTheDocument();
  });

  it('does not open when setup was already finished', async () => {
    api.onboardingState.mockResolvedValue({ ...fresh(), finished: true });
    render(App);
    expect(await screen.findByRole('navigation', { name: 'Main' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: /set up legaj/i })).not.toBeInTheDocument();
  });
});
