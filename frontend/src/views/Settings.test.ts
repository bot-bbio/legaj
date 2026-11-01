import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Settings, SettingsView } from '../lib/api';

vi.mock('../lib/events', () => ({ onEntityChanged: () => () => {} }));

const base: Settings = {
  model: 'gemini-3.1-flash-lite',
  skipReview: false,
  defaultMaterials: 'letter',
  followUpDays: 7,
  updateCheck: true,
  startWithWindows: false,
};
const view = (saved: boolean): SettingsView => ({
  settings: { ...base },
  key: { saved, masked: saved ? '••••WXYZ' : '' },
});

const api = {
  getSettings: vi.fn<() => Promise<SettingsView>>(),
  saveSettings: vi.fn<(s: Settings) => Promise<SettingsView>>(),
  saveApiKey: vi.fn<(k: string) => Promise<void>>(),
  removeApiKey: vi.fn<() => Promise<void>>(),
};
vi.mock('../lib/api', () => ({ api }));

const { default: SettingsView_ } = await import('./Settings.svelte');

const SECRET = 'AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ';

describe('R4 key field', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getSettings.mockResolvedValue(view(true));
    api.saveApiKey.mockResolvedValue();
    api.removeApiKey.mockResolvedValue();
  });

  it('shows only the mask, and the input is a password field with autocomplete off', async () => {
    render(SettingsView_);
    expect(await screen.findByText('••••WXYZ')).toBeInTheDocument();
    const input = screen.getByLabelText(/new api key/i);
    expect(input).toHaveAttribute('autocomplete', 'off');
    expect(input).toHaveAttribute('type', 'password');
    expect(document.body.textContent).not.toContain('SENTINEL');
  });

  it('Replace sends the key and clears it from the field', async () => {
    render(SettingsView_);
    const input = await screen.findByLabelText(/new api key/i);
    await userEvent.type(input, SECRET);
    await userEvent.click(screen.getByRole('button', { name: /replace key/i }));

    expect(api.saveApiKey).toHaveBeenCalledWith(SECRET);
    expect(input).toHaveValue('');
  });

  it('clears the key from the field even when saving fails', async () => {
    api.saveApiKey.mockRejectedValue('That doesn’t look like an API key.');
    render(SettingsView_);
    const input = await screen.findByLabelText(/new api key/i);
    await userEvent.type(input, 'bad');
    await userEvent.click(screen.getByRole('button', { name: /replace key/i }));

    expect(await screen.findByText('That doesn’t look like an API key.')).toBeInTheDocument();
    expect(input).toHaveValue('');
  });

  it('Remove asks for confirmation, then calls the backend', async () => {
    render(SettingsView_);
    await userEvent.click(await screen.findByRole('button', { name: /remove key/i }));
    expect(api.removeApiKey).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: /yes, remove/i }));
    expect(api.removeApiKey).toHaveBeenCalledTimes(1);
  });

  it('offers Save key (not Replace) when no key is saved', async () => {
    api.getSettings.mockResolvedValue(view(false));
    render(SettingsView_);
    expect(await screen.findByText(/no key saved/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save key/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /remove key/i })).not.toBeInTheDocument();
  });
});

describe('R37 preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.getSettings.mockResolvedValue(view(true));
    api.saveSettings.mockImplementation((s) => Promise.resolve({ settings: s, key: { saved: true, masked: '••••WXYZ' } }));
  });

  it('saves edited preferences', async () => {
    render(SettingsView_);
    const days = await screen.findByLabelText(/follow up after/i);
    await userEvent.clear(days);
    await userEvent.type(days, '10');
    await userEvent.selectOptions(screen.getByLabelText(/default materials/i), 'both');
    await userEvent.click(screen.getByLabelText(/start legaj with windows/i));
    await userEvent.click(screen.getByRole('button', { name: /save settings/i }));

    expect(api.saveSettings).toHaveBeenCalledWith({ ...base, followUpDays: 10, defaultMaterials: 'both', startWithWindows: true });
  });

  it('Save is disabled until something changes', async () => {
    render(SettingsView_);
    expect(await screen.findByRole('button', { name: /save settings/i })).toBeDisabled();
  });
});
