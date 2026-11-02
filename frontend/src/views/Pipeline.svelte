<script lang="ts">
  import BulkBar from '../components/BulkBar.svelte';
  import PasteJob from '../components/PasteJob.svelte';
  import StatsHeader from '../components/StatsHeader.svelte';
  import UndoToast from '../components/UndoToast.svelte';
  import VirtualTable from '../components/VirtualTable.svelte';
  import type { ApplicationRow, DeleteResult } from '../lib/api';
  import { filterRows, prune, toggle, type RowFilter } from '../lib/selection';
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { STAGES, stageLabel } from '../lib/stages';
  import { applicationsStore } from '../lib/stores/applications';

  let filter = $state<RowFilter>({ stage: 'all', query: '' });
  let selected = $state<Set<number>>(new Set());
  // Every delete still inside its undo window keeps its own toast (gate C8).
  let deletes = $state<DeleteResult[]>([]);

  function deleted(result: DeleteResult): void {
    selected = new Set();
    deletes = [...deletes, result];
  }

  function closeToast(undoId: string): void {
    deletes = deletes.filter((d) => d.undoId !== undoId);
  }

  let focusId = $state<number | null>(null);

  let pasting = $state(false);
  let openError = $state('');

  /** Links and files open only through the backend's safe opener (R33). */
  async function open(action: () => Promise<void>): Promise<void> {
    openError = '';
    try {
      await action();
    } catch (err) {
      openError = errorMessage(err);
    }
  }

  /** Shows an existing row: clears the filters, selects only that row and scrolls to it. */
  function show(id: number): void {
    pasting = false;
    filter = { stage: 'all', query: '' };
    selected = new Set([id]);
    focusId = id;
  }

  const rows = $derived($applicationsStore.status === 'ready' ? $applicationsStore.data : []);
  const visible = $derived(filterRows(rows, filter));
  const counts = $derived(
    rows.reduce<Record<string, number>>((acc, r) => ((acc[r.stage] = (acc[r.stage] ?? 0) + 1), acc), {}),
  );

  // A bulk action must never act on a row the user can't see: whenever the
  // filter or the data changes, hidden or deleted rows leave the selection.
  $effect(() => {
    const pruned = prune(selected, visible.map((r) => r.id));
    if (pruned.size !== selected.size) selected = pruned;
  });

  const allShownSelected = $derived(visible.length > 0 && visible.every((r) => selected.has(r.id)));
  const someShownSelected = $derived(!allShownSelected && visible.some((r) => selected.has(r.id)));

  function toggleAllShown(): void {
    // Exactly the visible IDs (R12, U2).
    selected = allShownSelected ? new Set() : new Set(visible.map((r) => r.id));
  }

  const added = (iso: string): string => new Date(iso).toLocaleDateString(undefined, { dateStyle: 'medium' });
  const rowLabel = (r: ApplicationRow): string => `Select ${r.company || 'row'}, ${r.role || 'no role'}`;
</script>

<div class="title">
  <h1>Pipeline</h1>
  {#if !pasting}
    <button type="button" onclick={() => (pasting = true)}>Paste job</button>
  {/if}
</div>

{#if pasting}
  <PasteJob onclose={() => (pasting = false)} onshow={show} />
{/if}

<StatsHeader />

{#if $applicationsStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $applicationsStore.status === 'error'}
  <p class="error">{$applicationsStore.message}</p>
{:else}
  <div class="toolbar">
    <label class="field">
      <span>Search</span>
      <input type="search" placeholder="Company or role" bind:value={filter.query} />
    </label>
    <label class="field">
      <span>Stage</span>
      <select bind:value={filter.stage}>
        <option value="all">All stages ({rows.length})</option>
        {#each STAGES as s (s.id)}
          <option value={s.id}>{s.label} ({counts[s.id] ?? 0})</option>
        {/each}
      </select>
    </label>
    <p class="muted summary">
      {visible.length} of {rows.length} shown
    </p>
  </div>

  {#if selected.size > 0}
    <BulkBar ids={[...selected]} ondeleted={deleted} />
  {/if}
  {#if openError}<p class="error" role="alert">{openError}</p>{/if}

  {#if rows.length === 0}
    <p class="muted">No applications yet. Clip a job from your browser, or use Paste job.</p>
  {:else}
    <VirtualTable rows={visible} label="Applications" scrollToId={focusId} onscrolled={() => (focusId = null)}>
      {#snippet header()}
        <span role="columnheader">
          <input
            type="checkbox"
            aria-label="Select all shown"
            checked={allShownSelected}
            indeterminate={someShownSelected}
            onchange={toggleAllShown}
          />
        </span>
        <span role="columnheader">Company</span>
        <span role="columnheader">Role</span>
        <span role="columnheader">Stage</span>
        <span role="columnheader">Location</span>
        <span role="columnheader">Added</span>
        <span role="columnheader">Open</span>
      {/snippet}
      {#snippet row(r: ApplicationRow)}
        <span role="cell">
          <input
            type="checkbox"
            aria-label={rowLabel(r)}
            data-id={r.id}
            checked={selected.has(r.id)}
            onchange={() => (selected = toggle(selected, r.id))}
          />
        </span>
        <span role="cell" class="clip">
          {#if r.company}{r.company}{:else}<span class="badge">Needs company</span>{/if}
        </span>
        <span role="cell" class="clip">{r.role}</span>
        <span role="cell"><span class="stage stage-{r.stage}">{stageLabel(r.stage)}</span></span>
        <span role="cell" class="clip muted">{r.location}</span>
        <span role="cell" class="muted">{added(r.createdAt)}</span>
        <span role="cell" class="links">
          {#if r.url}
            <button type="button" class="link" onclick={() => open(() => api.openPosting(r.id))}>Posting</button>
          {/if}
          <button type="button" class="link" onclick={() => open(() => api.openResume(r.id))}>Resume</button>
        </span>
      {/snippet}
    </VirtualTable>
  {/if}
{/if}

{#if deletes.length > 0}
  <div class="toasts">
    {#each deletes as d (d.undoId)}
      <UndoToast result={d} onclose={() => closeToast(d.undoId)} />
    {/each}
  </div>
{/if}

<style>
  .toasts {
    position: fixed;
    left: 50%;
    bottom: 1.5rem;
    transform: translateX(-50%);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    align-items: center;
  }
  .title {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .toolbar {
    display: flex;
    align-items: flex-end;
    gap: 1rem;
    flex-wrap: wrap;
    margin-bottom: 0.75rem;
  }
  .summary {
    margin: 0 0 0.6rem auto;
  }
  :global(.grid-row) {
    display: grid;
    grid-template-columns: 2.5rem minmax(8rem, 1fr) minmax(10rem, 1.5fr) 8rem minmax(6rem, 1fr) 7rem 8rem;
    align-items: center;
    gap: 0.75rem;
    padding: 0 0.75rem;
    min-height: 36px;
  }
  .links {
    display: flex;
    gap: 0.6rem;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    text-decoration: underline;
    cursor: pointer;
  }
  .clip {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--danger);
    font-size: 0.85em;
  }
  .stage {
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    background: var(--hover);
    font-size: 0.85em;
  }
  .stage-ready,
  .stage-offer {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .stage-rejected,
  .stage-withdrawn {
    color: var(--muted);
  }
</style>
