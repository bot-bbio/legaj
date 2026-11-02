<script lang="ts">
  import BulkBar from '../components/BulkBar.svelte';
  import PasteJob from '../components/PasteJob.svelte';
  import StatsHeader from '../components/StatsHeader.svelte';
  import UndoToast from '../components/UndoToast.svelte';
  import VirtualTable from '../components/VirtualTable.svelte';
  import type { ApplicationRow, DeleteResult } from '../lib/api';
  import { filterRows, prune, toggle, type RowFilter } from '../lib/selection';
  import { STAGES, stageLabel } from '../lib/stages';
  import { applicationsStore } from '../lib/stores/applications';

  let filter = $state<RowFilter>({ stage: 'all', query: '' });
  let selected = $state<Set<number>>(new Set());
  let lastDelete = $state<DeleteResult | null>(null);

  function deleted(result: DeleteResult): void {
    selected = new Set();
    lastDelete = result;
  }

  let pasting = $state(false);

  /** Shows an existing row: clears the filters and selects only that row. */
  function show(id: number): void {
    pasting = false;
    filter = { stage: 'all', query: '' };
    selected = new Set([id]);
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

  {#if rows.length === 0}
    <p class="muted">No applications yet. Clip a job from your browser, or use Paste job.</p>
  {:else}
    <VirtualTable rows={visible} label="Applications">
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
      {/snippet}
    </VirtualTable>
  {/if}
{/if}

{#if lastDelete}
  {#key lastDelete.undoId}
    <UndoToast result={lastDelete} onclose={() => (lastDelete = null)} />
  {/key}
{/if}

<style>
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
    grid-template-columns: 2.5rem minmax(8rem, 1fr) minmax(10rem, 1.5fr) 8rem minmax(6rem, 1fr) 7rem;
    align-items: center;
    gap: 0.75rem;
    padding: 0 0.75rem;
    min-height: 36px;
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
