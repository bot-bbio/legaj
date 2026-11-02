/**
 * The frontend's single doorway to Go. Every backend call goes through
 * `api`, so tests mock this one module. Types come from the Wails-generated
 * bindings (C13), minus the generated helper methods.
 */
import * as App from '../../wailsjs/go/main/App';
import * as OnboardingAPI from '../../wailsjs/go/main/OnboardingAPI';
import * as PipelineAPI from '../../wailsjs/go/main/PipelineAPI';
import * as ProfileAPI from '../../wailsjs/go/main/ProfileAPI';
import * as SettingsAPI from '../../wailsjs/go/main/SettingsAPI';
import { profile, type main, type onboarding, type pipeline, type settings } from '../../wailsjs/go/models';

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
export type OnboardingState = Plain<onboarding.State>;
export type KeyResult = Plain<onboarding.KeyResult>;
export type ResumeImport = Plain<main.ResumeImport>;
export type StepID = 'key' | 'resume' | 'profile' | 'clipper';
export type ApplicationRow = Plain<main.ApplicationRow>;
export type StageResult = Plain<pipeline.StageResult>;
export type DeleteResult = Plain<pipeline.DeleteResult>;
export type UndoResult = Plain<pipeline.UndoResult>;
export type StatsView = Plain<main.StatsView>;
export type ResumeFileView = Plain<main.ResumeFileView>;
export type PasteInput = Plain<pipeline.PasteInput>;
export type PasteResult = Plain<pipeline.PasteResult>;
export type Stage = 'lead' | 'ready' | 'applied' | 'interviewing' | 'offer' | 'rejected' | 'withdrawn';

export const api = {
  startup: (): Promise<StartupView> => App.Startup(),
  restoreBackup: (name: string): Promise<StartupView> => App.RestoreBackup(name),
  quit: (): Promise<void> => App.Quit(),
  version: (): Promise<string> => App.Version(),
  openURL: (link: string): Promise<void> => App.OpenURL(link),

  getSettings: (): Promise<SettingsView> => SettingsAPI.GetSettings(),
  saveSettings: (s: Settings): Promise<SettingsView> => SettingsAPI.SaveSettings(s),
  saveApiKey: (key: string): Promise<void> => SettingsAPI.SaveAPIKey(key),
  removeApiKey: (): Promise<void> => SettingsAPI.RemoveAPIKey(),

  getProfile: (): Promise<Profile> => ProfileAPI.GetProfile(),
  saveProfile: (p: Profile): Promise<Profile> => ProfileAPI.SaveProfile(profile.Profile.createFrom(p)),
  currentResume: (): Promise<ResumeFileView> => ProfileAPI.CurrentResume(),
  uploadNewResume: (): Promise<ResumeImport> => ProfileAPI.UploadNewResume(),
  openCurrentResume: (): Promise<void> => ProfileAPI.OpenCurrentResume(),

  onboardingState: (): Promise<OnboardingState> => OnboardingAPI.OnboardingState(),
  saveKeyAndTest: (key: string): Promise<KeyResult> => OnboardingAPI.SaveKey(key),
  uploadResume: (): Promise<ResumeImport> => OnboardingAPI.UploadResume(),
  parseResume: (fileId: number): Promise<ResumeImport> => OnboardingAPI.ParseResume(fileId),
  finishOnboarding: (): Promise<void> => OnboardingAPI.FinishOnboarding(),

  listApplications: (): Promise<ApplicationRow[]> => PipelineAPI.ListApplications(),
  setStage: (ids: number[], stage: string): Promise<StageResult> => PipelineAPI.SetStage(ids, stage),
  reopen: (ids: number[], stage: string): Promise<StageResult> => PipelineAPI.Reopen(ids, stage),
  deleteApplications: (ids: number[]): Promise<DeleteResult> => PipelineAPI.DeleteApplications(ids),
  undoDelete: (undoId: string): Promise<UndoResult> => PipelineAPI.UndoDelete(undoId),
  pipelineStats: (): Promise<StatsView> => PipelineAPI.PipelineStats(),
  pasteJob: (input: PasteInput): Promise<PasteResult> => PipelineAPI.PasteJob(input),
  openPosting: (id: number): Promise<void> => PipelineAPI.OpenPosting(id),
  openResume: (id: number): Promise<void> => PipelineAPI.OpenResume(id),
};
