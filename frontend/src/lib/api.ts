/**
 * The frontend's single doorway to Go. Every backend call goes through
 * `api`, so tests mock this one module. Types come from the Wails-generated
 * bindings (C13), minus the generated helper methods.
 */
import * as AnswersAPI from '../../wailsjs/go/main/AnswersAPI';
import * as App from '../../wailsjs/go/main/App';
import * as FollowupsAPI from '../../wailsjs/go/main/FollowupsAPI';
import * as InterviewsAPI from '../../wailsjs/go/main/InterviewsAPI';
import * as LettersAPI from '../../wailsjs/go/main/LettersAPI';
import * as OnboardingAPI from '../../wailsjs/go/main/OnboardingAPI';
import * as PipelineAPI from '../../wailsjs/go/main/PipelineAPI';
import * as ProfileAPI from '../../wailsjs/go/main/ProfileAPI';
import * as SettingsAPI from '../../wailsjs/go/main/SettingsAPI';
import { ClipboardSetText } from '../../wailsjs/runtime/runtime';
import {
  followups,
  interviews,
  letters,
  profile,
  type answers,
  type documents,
  type main,
  type onboarding,
  type pipeline,
  type settings,
  type store,
  type update,
} from '../../wailsjs/go/models';

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
export type Draft = Plain<letters.Draft>;
export type DocumentVersion = Plain<documents.Version>;
export type DraftPart = 'opening' | 'why' | 'body' | 'closing';
export type ReviewItem = Plain<main.ReviewItem>;
export type ReviewStatus = 'queued' | 'running' | 'draft' | 'failed';
export type GenerateResult = Plain<main.GenerateResult>;
export type ApproveAllResult = Plain<letters.ApproveAllResult>;
export type Answer = Plain<answers.Answer>;
export type Suggestion = Plain<answers.Suggestion>;
export type DueView = Plain<main.DueView>;
export type FollowupDraft = Plain<followups.DraftView>;
export type FollowupInput = Plain<followups.DraftInput>;
export type FollowupKind = 'applied_nudge' | 'thank_you';
export type MailClient = 'gmail' | 'outlook' | 'outlook365' | 'mailto';
export type RoundView = Plain<main.RoundView>;
export type RoundLists = Plain<main.RoundLists>;
export type RoundInput = Plain<interviews.RoundInput>;
export type Interviewer = Plain<store.Interviewer>;
export type UpdateResult = Plain<update.Result>;
export type PasteInput = Plain<pipeline.PasteInput>;
export type PasteResult = Plain<pipeline.PasteResult>;
export type Stage = 'lead' | 'ready' | 'applied' | 'interviewing' | 'offer' | 'rejected' | 'withdrawn';

export const api = {
  startup: (): Promise<StartupView> => App.Startup(),
  restoreBackup: (name: string): Promise<StartupView> => App.RestoreBackup(name),
  quit: (): Promise<void> => App.Quit(),
  version: (): Promise<string> => App.Version(),
  openURL: (link: string): Promise<void> => App.OpenURL(link),
  /** Notify-only release check (R40); no request when Settings turns it off. */
  checkForUpdate: (): Promise<UpdateResult> => App.CheckForUpdate(),

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

  getDraft: (appId: number): Promise<Draft> => LettersAPI.GetDraft(appId),
  saveDraft: (appId: number, d: Draft): Promise<Draft> => LettersAPI.SaveDraft(appId, letters.Draft.createFrom(d)),
  regenerateParagraph: (appId: number, part: DraftPart, index: number): Promise<Draft> =>
    LettersAPI.RegenerateParagraph(appId, part, index),
  approveLetter: (appId: number): Promise<DocumentVersion> => LettersAPI.ApproveLetter(appId),
  /** Opens the newest approved letter (PDF) in the default viewer. */
  openLetter: (appId: number): Promise<void> => LettersAPI.OpenLetter(appId),
  /** Opens the folder holding an application's letters, to attach one on a job site. */
  showLetterFolder: (appId: number): Promise<void> => LettersAPI.ShowLetterFolder(appId),
  generateLetters: (appIds: number[]): Promise<GenerateResult> => LettersAPI.Generate(appIds),
  approveAll: (appIds: number[]): Promise<ApproveAllResult> => LettersAPI.ApproveAll(appIds),
  reviewQueue: (): Promise<ReviewItem[]> => LettersAPI.ReviewQueue(),
  dismissJob: (appId: number): Promise<void> => LettersAPI.Dismiss(appId),
  setMaterials: (id: number, materials: Materials): Promise<ApplicationRow> => PipelineAPI.SetMaterials(id, materials),

  generateAnswers: (appId: number, question: string): Promise<Answer> => AnswersAPI.Generate(appId, question),
  listAnswers: (appId: number): Promise<Answer[]> => AnswersAPI.List(appId),
  approveAnswer: (id: number, text: string): Promise<Answer> => AnswersAPI.Approve(id, text),
  deleteAnswer: (id: number): Promise<void> => AnswersAPI.Delete(id),
  answerBank: (): Promise<Answer[]> => AnswersAPI.Bank(),
  suggestAnswers: (question: string): Promise<Suggestion[]> => AnswersAPI.Suggest(question),
  removeFromBank: (id: number): Promise<void> => AnswersAPI.RemoveFromBank(id),

  followupsDue: (): Promise<DueView[]> => FollowupsAPI.Due(),
  followupDraft: (kind: FollowupKind, appId: number, roundId: number): Promise<FollowupDraft> =>
    FollowupsAPI.Draft(kind, appId, roundId),
  saveFollowupDraft: (d: FollowupInput): Promise<FollowupDraft> =>
    FollowupsAPI.SaveDraft(followups.DraftInput.createFrom(d)),
  /** Opens a compose window in the user's mail client; nothing is sent from LeGaJ (D10). */
  openCompose: (d: FollowupInput, client: MailClient): Promise<void> =>
    FollowupsAPI.OpenCompose(followups.DraftInput.createFrom(d), client),
  logFollowup: (d: FollowupInput, status: 'sent' | 'skipped'): Promise<void> =>
    FollowupsAPI.Log(followups.DraftInput.createFrom(d), status),

  interviewLists: (): Promise<RoundLists> => InterviewsAPI.Lists(),
  addRound: (r: RoundInput): Promise<RoundView> => InterviewsAPI.Add(interviews.RoundInput.createFrom(r)),
  updateRound: (id: number, r: RoundInput): Promise<RoundView> =>
    InterviewsAPI.Update(id, interviews.RoundInput.createFrom(r)),
  deleteRound: (id: number): Promise<void> => InterviewsAPI.Delete(id),
  /** Writes the round's .ics file and opens it in the default calendar app (R31). */
  addToCalendar: (id: number): Promise<void> => InterviewsAPI.AddToCalendar(id),
  suggestQuestions: (roundId: number): Promise<RoundView> => InterviewsAPI.SuggestQuestions(roundId),

  /** Puts text on the system clipboard through the Wails runtime. */
  copyText: async (text: string): Promise<void> => {
    if (!(await ClipboardSetText(text))) throw new Error('LeGaJ couldn’t copy to the clipboard.');
  },
};
