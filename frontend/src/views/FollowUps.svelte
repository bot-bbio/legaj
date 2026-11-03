<script lang="ts">
  /**
   * Follow-ups (R27-R29, D10): what's due, a draft for each, and a compose
   * window in the user's own mail client. LeGaJ never sends anything; after
   * opening a draft it asks "Sent?" and logs the answer.
   */
  import { tick } from 'svelte';

  import {
    api,
    type DueView,
    type FollowupDraft,
    type FollowupInput,
    type FollowupKind,
    type MailClient,
  } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { dueStore } from '../lib/stores/due';

  const CLIENTS: readonly { id: MailClient; label: string; action: string }[] = [
    { id: 'gmail', label: 'Gmail', action: 'Open in Gmail' },
    { id: 'outlook', label: 'Outlook.com', action: 'Open in Outlook.com' },
    { id: 'outlook365', label: 'Outlook (work or school)', action: 'Open in Outlook' },
    { id: 'mailto', label: 'My mail app', action: 'Open in your mail app' },
  ];
  const CLIENT_KEY = 'legaj.mailClient';

  function savedClient(): MailClient {
    try {
      const v = localStorage.getItem(CLIENT_KEY);
      const found = CLIENTS.find((c) => c.id === v);
      if (found) return found.id;
    } catch {
      // Storage unavailable: use the default.
    }
    return 'gmail';
  }

  let client = $state<MailClient>(savedClient());
  $effect(() => {
    try {
      localStorage.setItem(CLIENT_KEY, client);
    } catch {
      // A per-device convenience only.
    }
  });
  const clientInfo = $derived(CLIENTS.find((c) => c.id === client) ?? CLIENTS[0]);

  const due = $derived($dueStore.status === 'ready' ? $dueStore.data : []);

  const keyOf = (d: Pick<DueView, 'kind' | 'appId' | 'roundId'>): string => `${d.kind}:${d.appId}:${d.roundId}`;
  const isKind = (k: string): k is FollowupKind => k === 'applied_nudge' || k === 'thank_you';
  const kindLabel = (k: string): string => (k === 'thank_you' ? 'Thank-you' : 'Follow-up');
  const name = (d: DueView): string => d.company || d.role || 'Untitled application';
  const input = (d: FollowupDraft): FollowupInput => ({
    kind: d.kind,
    appId: d.appId,
    roundId: d.roundId,
    to: d.to,
    subject: d.subject,
    body: d.body,
  });

  let drafts = $state<Record<string, FollowupDraft>>({});
  let editing = $state<Record<string, boolean>>({});
  // Drafts opened in the mail client, waiting for the "Sent?" answer.
  let opened = $state<Record<string, boolean>>({});
  let itemErrors = $state<Record<string, string>>({});
  let busy = $state(false);
  let confirming = $state(false);
  let notes = $state<string[]>([]);
  let dialog = $state<HTMLElement | null>(null);

  async function ensureDraft(d: DueView): Promise<FollowupDraft | null> {
    const key = keyOf(d);
    const have = drafts[key];
    if (have) return have;
    if (!isKind(d.kind)) return null;
    const loaded = await api.followupDraft(d.kind, d.appId, d.roundId);
    drafts[key] = loaded;
    return loaded;
  }

  async function write(d: DueView): Promise<void> {
    const key = keyOf(d);
    itemErrors[key] = '';
    try {
      await ensureDraft(d);
      editing[key] = !editing[key];
    } catch (err) {
      itemErrors[key] = errorMessage(err);
    }
  }

  /** Saves edits and refreshes the draft's problems and warnings. */
  async function save(key: string): Promise<void> {
    const d = drafts[key];
    if (!d) return;
    try {
      const v = await api.saveFollowupDraft(input(d));
      const current = drafts[key];
      if (current) {
        current.problems = v.problems;
        current.warnings = v.warnings;
      }
      itemErrors[key] = '';
    } catch (err) {
      itemErrors[key] = errorMessage(err);
    }
  }

  async function openOne(d: DueView): Promise<void> {
    const key = keyOf(d);
    const draft = drafts[key];
    if (!draft) return;
    busy = true;
    itemErrors[key] = '';
    try {
      await api.openCompose(input(draft), client);
      opened[key] = true;
    } catch (err) {
      itemErrors[key] = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function askOpenAll(): Promise<void> {
    confirming = true;
    notes = [];
    await tick();
    dialog?.focus();
  }

  /** Opens every due draft that's ready; flagged ones are listed instead (R27). */
  async function openAll(): Promise<void> {
    confirming = false;
    busy = true;
    const skipped: string[] = [];
    try {
      for (const d of due) {
        const key = keyOf(d);
        try {
          const draft = await ensureDraft(d);
          if (!draft) continue;
          if (draft.problems.length > 0) {
            skipped.push(`${name(d)}: ${draft.problems[0] ?? ''}`);
            continue;
          }
          await api.openCompose(input(draft), client);
          opened[key] = true;
        } catch (err) {
          skipped.push(`${name(d)}: ${errorMessage(err)}`);
        }
      }
    } finally {
      busy = false;
      notes = skipped;
    }
  }

  async function log(d: DueView, status: 'sent' | 'skipped'): Promise<void> {
    const key = keyOf(d);
    busy = true;
    itemErrors[key] = '';
    try {
      const draft = drafts[key];
      const fallback: FollowupInput = {
        kind: d.kind,
        appId: d.appId,
        roundId: d.roundId,
        to: '',
        subject: '',
        body: '',
      };
      await api.logFollowup(draft ? input(draft) : fallback, status);
      opened[key] = false;
      await dueStore.refresh();
    } catch (err) {
      itemErrors[key] = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const when = (iso: string): string =>
    new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`;
</script>

<div class="title">
  <h1>Follow-ups</h1>
  <label class="field client">
    <span>Send with</span>
    <select bind:value={client}>
      {#each CLIENTS as c (c.id)}
        <option value={c.id}>{c.label}</option>
      {/each}
    </select>
  </label>
</div>
<p class="muted">LeGaJ opens a draft in your mail client; you press Send there. It never sends email itself.</p>

{#if $dueStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $dueStore.status === 'error'}
  <p class="error">{$dueStore.message}</p>
{:else if due.length === 0}
  <p class="muted">Nothing is due. Follow-ups appear here after you apply and after each interview.</p>
{:else}
  <div class="bar">
    <button type="button" disabled={busy || confirming} onclick={askOpenAll}>Open all due ({due.length})</button>
  </div>

  {#if confirming}
    <div
      class="card confirm"
      role="alertdialog"
      aria-labelledby="open-all-title"
      aria-describedby="open-all-text"
      tabindex="-1"
      bind:this={dialog}
    >
      <h2 id="open-all-title">Open all due follow-ups?</h2>
      <p id="open-all-text">
        {plural(due.length, 'compose window')} will open in {clientInfo?.label}. Nothing is sent until you press Send
        there. Drafts that still need fixing are skipped.
      </p>
      <div class="actions">
        <button type="button" onclick={openAll}>Open all</button>
        <button type="button" class="secondary" onclick={() => (confirming = false)}>Cancel</button>
      </div>
    </div>
  {/if}

  {#if notes.length > 0}
    <div class="notes" role="status">
      <p>Not opened:</p>
      <ul>
        {#each notes as n, i (i)}<li>{n}</li>{/each}
      </ul>
    </div>
  {/if}

  {#each due as d (keyOf(d))}
    {@const key = keyOf(d)}
    {@const draft = drafts[key]}
    <article class="card item" aria-label="{kindLabel(d.kind)}: {name(d)}">
      <div class="head">
        <p>
          <strong>{kindLabel(d.kind)}: {name(d)}</strong>
          <span class="muted">{d.role}</span>
        </p>
        <p class="muted">
          {#if d.kind === 'thank_you'}
            Interview ended {when(d.dueSince)}{d.interviewer ? ` · ${d.interviewer}` : ''}
          {:else}
            Applied {plural(d.daysSince, 'day')} ago
          {/if}
        </p>
      </div>

      {#if opened[key]}
        <div class="sent" role="group" aria-label="Sent?">
          <span>Sent?</span>
          <button type="button" disabled={busy} onclick={() => log(d, 'sent')}>Sent</button>
          <button type="button" class="secondary" disabled={busy} onclick={() => (opened[key] = false)}>Not yet</button>
          <button type="button" class="secondary" disabled={busy} onclick={() => log(d, 'skipped')}>Skip it</button>
        </div>
      {/if}

      <div class="actions">
        <button type="button" class="secondary" aria-expanded={!!editing[key]} onclick={() => write(d)}>
          {editing[key] ? 'Hide draft' : 'Write'}
        </button>
        {#if !opened[key]}
          <button type="button" class="secondary" disabled={busy} onclick={() => log(d, 'skipped')}>Skip</button>
        {/if}
      </div>

      {#if editing[key] && draft}
        <div class="draft">
          <label class="field">
            <span>To (optional for web mail)</span>
            <input type="email" autocomplete="off" bind:value={draft.to} onchange={() => save(key)} />
          </label>
          <label class="field">
            <span>Subject</span>
            <input type="text" bind:value={draft.subject} onchange={() => save(key)} />
          </label>
          <label class="field">
            <span>Message</span>
            <textarea rows="9" bind:value={draft.body} onchange={() => save(key)}></textarea>
          </label>
          {#each draft.problems as p, i (i)}<p class="error">{p}</p>{/each}
          {#each draft.warnings as w, i (i)}<p class="warning">{w}</p>{/each}
          <div class="actions">
            <button type="button" disabled={busy || draft.problems.length > 0} onclick={() => openOne(d)}>
              {clientInfo?.action}
            </button>
          </div>
        </div>
      {/if}

      {#if itemErrors[key]}<p class="error" role="alert">{itemErrors[key]}</p>{/if}
    </article>
  {/each}
{/if}

<style>
  .title {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
  }
  .client {
    min-width: 14rem;
  }
  .bar {
    margin-bottom: 0.75rem;
  }
  .confirm {
    max-width: 40rem;
    margin-bottom: 0.75rem;
    border-color: var(--accent);
  }
  .confirm h2 {
    margin-top: 0;
  }
  .notes {
    max-width: 56rem;
    margin-bottom: 0.75rem;
  }
  .item {
    display: grid;
    gap: 0.5rem;
    max-width: 56rem;
    margin-bottom: 0.75rem;
  }
  .head p {
    margin: 0;
  }
  .actions,
  .sent {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .sent {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
  .draft {
    display: grid;
    gap: 0.5rem;
  }
  .warning {
    color: var(--muted);
  }
</style>
