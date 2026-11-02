/**
 * The frontend's single doorway to Go. Every backend call goes through
 * `api`, so tests mock this one module. Types come from the Wails-generated
 * bindings (C13), minus the generated helper methods.
 */
import * as App from '../../wailsjs/go/main/App';
import * as ProfileAPI from '../../wailsjs/go/main/ProfileAPI';
import * as SettingsAPI from '../../wailsjs/go/main/SettingsAPI';
import { profile, type main, type settings } from '../../wailsjs/go/models';

/** The data fields of a generated model class (deeply), without its helper methods. */
export type Plain<T> = T extends readonly (infer U)[]
  ? Plain<U>[]
  : T extends object
    ? { [K in keyof T as T[K] extends (...args: never[]) => unknown ? never : K]: Plain<T[K]> }
    : T;

export type StartupView = Plain<main.StartupView>;
export type BackupView = Plain<main.BackupView>;
export type Settings = Plain<settings.Settings>;
export type SettingsView = Plain<main.SettingsView>;
export type Materials = 'letter' | 'answers' | 'both';
export type Profile = Plain<profile.Profile>;
export type Experience = Plain<profile.Experience>;
export type Education = Plain<profile.Education>;
export type SkillGroup = Plain<profile.SkillGroup>;
export type YearMonth = Plain<profile.YearMonth>;

export const api = {
  startup: (): Promise<StartupView> => App.Startup(),
  restoreBackup: (name: string): Promise<StartupView> => App.RestoreBackup(name),
  quit: (): Promise<void> => App.Quit(),
  version: (): Promise<string> => App.Version(),

  getSettings: (): Promise<SettingsView> => SettingsAPI.GetSettings(),
  saveSettings: (s: Settings): Promise<SettingsView> => SettingsAPI.SaveSettings(s),
  saveApiKey: (key: string): Promise<void> => SettingsAPI.SaveAPIKey(key),
  removeApiKey: (): Promise<void> => SettingsAPI.RemoveAPIKey(),

  getProfile: (): Promise<Profile> => ProfileAPI.GetProfile(),
  saveProfile: (p: Profile): Promise<Profile> => ProfileAPI.SaveProfile(profile.Profile.createFrom(p)),
};
