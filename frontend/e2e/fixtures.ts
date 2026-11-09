/**
 * Data for the fake backend: a finished setup and a pipeline full of
 * realistic rows, with long names that stress the layout the way real job
 * titles do. All of it is made up.
 */
import type { ApplicationRow, StatsView } from '../src/lib/api';
import type { FakeData } from './fake-backend';

const companies = [
  'Northwind Logistics',
  'Contoso Health Systems International',
  'Fabrikam',
  'Wide World Importers & Distribution Co.',
  'Tailspin Toys',
  'Adventure Works Cycles',
  'Woodgrove Bank',
  'Lucerne Publishing Group',
];
const roles = [
  'Senior Data Analyst, Supply Chain Forecasting and Planning',
  'Business Intelligence Developer II',
  'Analyst',
  'Product Analyst – Growth & Monetization (Remote, North America)',
  'Junior Data Scientist',
  'Reporting Specialist',
];
const locations = ['Springfield, IL', 'Remote', 'Toronto, ON, Canada', 'New York, NY (Hybrid, 3 days in office)'];
const stages = ['lead', 'ready', 'applied', 'interviewing', 'offer', 'rejected', 'withdrawn'] as const;

/** n made-up applications, newest first. */
export function applications(n: number): ApplicationRow[] {
  return Array.from({ length: n }, (_, i): ApplicationRow => {
    const day = String(1 + (i % 28)).padStart(2, '0');
    const created = `2026-10-${day}T15:04:05Z`;
    const stage = stages[i % stages.length] ?? 'lead';
    return {
      id: i + 1,
      company: i % 11 === 5 ? '' : (companies[i % companies.length] ?? ''),
      role: roles[i % roles.length] ?? '',
      location: locations[i % locations.length] ?? '',
      url: `https://jobs.example.com/posting/${i + 1}`,
      stage,
      source: i % 2 === 0 ? 'clip' : 'paste',
      materials: 'letter',
      letterVersion: i % 3 === 0 ? 1 : 0,
      createdAt: created,
      appliedAt: stage === 'lead' || stage === 'ready' ? '' : created,
    };
  });
}

function statsFor(rows: ApplicationRow[]): StatsView {
  const counts: Record<string, number> = {};
  for (const r of rows) counts[r.stage] = (counts[r.stage] ?? 0) + 1;
  return {
    counts,
    total: rows.length,
    recent: rows.slice(0, 5).map((r) => ({ id: r.id, company: r.company, role: r.role, stage: r.stage, createdAt: r.createdAt })),
  };
}

/** A finished setup with `rows` applications. */
export function baseData(rows = 60): FakeData {
  const apps = applications(rows);
  return {
    startup: { kind: 'ready', movedAside: '', backups: [], message: '' },
    version: '3.0.0-e2e',
    update: { available: false, latest: '', url: '' },
    clip: { running: true, port: 47123, needsReinstall: false, message: '' },
    settings: {
      settings: {
        model: 'gemini-3.1-flash-lite',
        skipReview: false,
        defaultMaterials: 'letter',
        followUpDays: 7,
        updateCheck: false,
        startWithWindows: false,
      },
      key: { saved: true, masked: '••••WXYZ' },
    },
    profile: {
      fullName: 'Alex Rivera',
      email: 'alex.rivera@example.com',
      phone: '(555) 010-0199',
      location: 'Springfield, IL',
      headline: 'Data analyst',
      targetRole: 'Data Analyst',
      summary: 'Four years turning operational data into forecasts.',
      links: [],
      experiences: [],
      education: [],
      skills: [],
    },
    resume: { id: 1, fileName: 'Alex Rivera Resume.pdf', sizeBytes: 92_000, uploadedAt: '2026-10-01T12:00:00Z' },
    onboarding: {
      finished: true,
      steps: [
        { id: 'key', done: true },
        { id: 'resume', done: true },
        { id: 'profile', done: true },
        { id: 'clipper', done: true },
      ],
    },
    applications: apps,
    stats: statsFor(apps),
    reviewQueue: [],
    bank: [],
    due: [],
    rounds: { upcoming: [], past: [] },
  };
}
