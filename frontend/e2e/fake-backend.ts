/**
 * A stand-in for LeGaJ's Go backend in the real-browser layout suite (R70).
 *
 * The Wails bindings call `window.go.main.<API>.<Method>()` and
 * `window.runtime.*`, so the suite serves the real production build and
 * defines those objects before the app loads. Reads return the typed data
 * below. A call the fake doesn't handle is rejected and recorded in
 * `window.__legajUnhandled`, which `expectAllHandled` turns into a failure
 * naming the method, instead of a mysterious blank screen.
 */
import { expect, type Page } from '@playwright/test';

import type {
  Answer,
  ApplicationRow,
  ClipStatus,
  DueView,
  OnboardingState,
  Profile,
  ResumeFileView,
  ReviewItem,
  RoundLists,
  SettingsView,
  StartupView,
  StatsView,
  UpdateResult,
} from '../src/lib/api';

/** Everything the fake backend answers. */
export interface FakeData {
  startup: StartupView;
  version: string;
  update: UpdateResult;
  clip: ClipStatus;
  settings: SettingsView;
  profile: Profile;
  resume: ResumeFileView;
  onboarding: OnboardingState;
  applications: ApplicationRow[];
  stats: StatsView;
  reviewQueue: ReviewItem[];
  bank: Answer[];
  due: DueView[];
  rounds: RoundLists;
}

/** Installs the fake before any page script runs. */
export async function installFakeBackend(page: Page, data: FakeData): Promise<void> {
  // This function is serialized into the page: it may use only its argument.
  await page.addInitScript((d: FakeData) => {
    type Handler = () => unknown;
    const reads: Record<string, Record<string, Handler>> = {
      App: {
        Startup: () => d.startup,
        Version: () => d.version,
        CheckForUpdate: () => d.update,
        ClipStatus: () => d.clip,
      },
      SettingsAPI: { GetSettings: () => d.settings },
      ProfileAPI: { GetProfile: () => d.profile, CurrentResume: () => d.resume },
      OnboardingAPI: { OnboardingState: () => d.onboarding },
      PipelineAPI: { ListApplications: () => d.applications, PipelineStats: () => d.stats },
      LettersAPI: { ReviewQueue: () => d.reviewQueue },
      AnswersAPI: { Bank: () => d.bank },
      FollowupsAPI: { Due: () => d.due },
      InterviewsAPI: { Lists: () => d.rounds },
    };
    const unhandled: string[] = [];
    const binding = (api: string): unknown =>
      new Proxy(
        {},
        {
          get: (_target, method) => {
            if (typeof method !== 'string') return undefined;
            return () => {
              const handler = reads[api]?.[method];
              if (!handler) {
                unhandled.push(`${api}.${method}`);
                return Promise.reject(new Error(`fake backend: ${api}.${method} is not handled`));
              }
              return Promise.resolve(structuredClone(handler()));
            };
          },
        },
      );
    const apis = [
      'App',
      'SettingsAPI',
      'ProfileAPI',
      'OnboardingAPI',
      'PipelineAPI',
      'LettersAPI',
      'AnswersAPI',
      'FollowupsAPI',
      'InterviewsAPI',
    ];
    // Runtime calls (events, clipboard, logging) all succeed and do nothing.
    const runtime = new Proxy(
      {},
      {
        get: (_target, name) => {
          if (typeof name !== 'string') return undefined;
          if (name.startsWith('EventsOn')) return () => () => undefined;
          return () => Promise.resolve(true);
        },
      },
    );
    const w = window as unknown as Record<string, unknown>;
    w['go'] = { main: Object.fromEntries(apis.map((api) => [api, binding(api)])) };
    w['runtime'] = runtime;
    w['__legajUnhandled'] = unhandled;
  }, data);
}

/** Fails the test if the app called a method the fake doesn't handle. */
export async function expectAllHandled(page: Page): Promise<void> {
  const unhandled = await page.evaluate(
    () => (window as unknown as { __legajUnhandled?: string[] }).__legajUnhandled ?? [],
  );
  expect(unhandled, 'backend calls the fake backend does not handle').toEqual([]);
}
