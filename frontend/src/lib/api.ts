/**
 * The frontend's single doorway to Go. Every backend call goes through
 * `api`, so tests mock this one module. Types come from the Wails-generated
 * bindings (C13), minus the generated helper methods.
 */
import * as App from '../../wailsjs/go/main/App';
import * as SettingsAPI from '../../wailsjs/go/main/SettingsAPI';
import type { main, settings } from '../../wailsjs/go/models';

/** The data fields of a generated model class, without its methods. */
export type Plain<T> = {
  [K in keyof T as T[K] extends (...args: never[]) => unknown ? never : K]: T[K];
};

export type StartupView = Plain<main.StartupView>;
export type BackupView = Plain<main.BackupView>;
export type Settings = Plain<settings.Settings>;
export type SettingsView = Plain<main.SettingsView>;
export type Materials = 'letter' | 'answers' | 'both';

export const api = {
  startup: (): Promise<StartupView> => App.Startup(),
  restoreBackup: (name: string): Promise<StartupView> => App.RestoreBackup(name),
  quit: (): Promise<void> => App.Quit(),
  version: (): Promise<string> => App.Version(),

  getSettings: (): Promise<SettingsView> => SettingsAPI.GetSettings(),
  saveSettings: (s: Settings): Promise<SettingsView> => SettingsAPI.SaveSettings(s),
  saveApiKey: (key: string): Promise<void> => SettingsAPI.SaveAPIKey(key),
  removeApiKey: (): Promise<void> => SettingsAPI.RemoveAPIKey(),
};
