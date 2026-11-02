<script lang="ts">
  /**
   * The Review panel (R23, R43): a side panel inside Pipeline, never a
   * separate screen. It follows background generation, and each draft can
   * be approved in one click, edited first, or all approved together. A
   * failure stays listed with its reason until retried or dismissed.
   */
  import LetterEditor from './LetterEditor.svelte';
  import { api, type DocumentVersion, type ReviewItem } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { reviewStore } from '../lib/stores/review';

  let { onclose }: { onclose: () => void } = $props();

  let busy = $state(false);
  let editing = $state<number | null>(null);
  // Outcomes of the latest actions; items themselves leave the list once approved.
  let notices = $state<string[]>([]);
  let failures = $state<string[]>([]);

  const items = $derived($reviewStore.status === 'ready' ? $reviewStore.data : []);
  const drafts = $derived(items.filter((i) => i.status === 'draft'));

  const name = (i: Pick<ReviewItem, 'company' | 'role'>): string => i.company || i.role || 'Untitled application';
  const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`;

  function report(item: ReviewItem, v: DocumentVersion): void {
    notices = [`${name(item)}: saved as version ${v.version}.`];
    failures = [];
    if (editing === item.appId) editing = null;
  }

  async function act(fn: () => Promise<void>): Promise<void> {
    busy = true;
    try {
      await fn();
    } catch (err) {
      notices = [];
      failures = [errorMessage(err)];
    } finally {
      busy = false;
    }
  }

  const approve = (item: ReviewItem): Promise<void> =>
    act(async () => report(item, await api.approveLetter(item.appId)));

  const approveAll = (): Promise<void> =>
    act(async () => {
      const byId = new Map(items.map((i) => [i.appId, i]));
      const r = await api.approveAll(drafts.map((d) => d.appId));
      notices = [`Approved ${plural(r.approved.length, 'letter')}.`];
      failures = r.failed.map((f) => {
        const it = byId.get(f.appId);
        return `${it ? name(it) : 'An application'}: ${f.reason}`;
      });
      editing = null;
    });

  const retry = (item: ReviewItem): Promise<void> =>
    act(async () => {
      await api.generateLetters([item.appId]);
    });

  const dismiss = (item: ReviewItem): Promise<void> => act(() => api.dismissJob(item.appId));
</script>

<div class="card review" role="complementary" aria-label="Review">
  <div class="head">
    <h2>Review</h2>
    {#if drafts.length > 0}
      <button type="button" disabled={busy} onclick={approveAll}>Approve all ({drafts.length})</button>
    {/if}
    <button type="button" class="secondary" onclick={onclose}>Close</button>
  </div>

  {#if notices.length > 0 || failures.length > 0}
    <div role="status">
      {#each notices as n, i (i)}<p class="notice">{n}</p>{/each}
      {#each failures as f, i (i)}<p class="error">{f}</p>{/each}
    </div>
  {/if}

  {#if $reviewStore.status === 'loading'}
    <p class="muted">Loading…</p>
  {:else if $reviewStore.status === 'error'}
    <p class="error">{$reviewStore.message}</p>
  {:else if items.length === 0}
    <p class="muted">Nothing to review. Use Generate on a row to draft its cover letter.</p>
  {:else}
    <ul>
      {#each items as it (it.appId)}
        <li aria-label="{name(it)}, {it.role}">
          <p class="title"><strong>{name(it)}</strong> <span class="muted">{it.role}</span></p>
          {#if it.status === 'queued'}
            <p class="muted">Waiting to start…</p>
          {:else if it.status === 'running'}
            <p class="muted">Drafting…</p>
          {:else if it.status === 'failed'}
            <p class="error">{it.reason}</p>
            <div class="actions">
              <button type="button" disabled={busy} onclick={() => retry(it)}>Retry</button>
              <button type="button" class="secondary" disabled={busy} onclick={() => dismiss(it)}>Dismiss</button>
            </div>
          {:else}
            {#if it.reason}<p class="error">{it.reason}</p>{/if}
            <div class="actions">
              <button type="button" disabled={busy} onclick={() => approve(it)}>Approve</button>
              <button
                type="button"
                class="secondary"
                aria-expanded={editing === it.appId}
                onclick={() => (editing = editing === it.appId ? null : it.appId)}
              >
                {editing === it.appId ? 'Hide editor' : 'Edit letter'}
              </button>
            </div>
            {#if editing === it.appId}
              <LetterEditor appId={it.appId} onapproved={(v) => report(it, v)} />
            {/if}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .review {
    display: grid;
    gap: 0.5rem;
    align-content: start;
    max-height: calc(100vh - 6rem);
    overflow-y: auto;
  }
  .head {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .head h2 {
    margin: 0 auto 0 0;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.5rem;
  }
  li {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
  }
  .title {
    margin: 0 0 0.25rem;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
  }
  .notice {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
</style>
