<script lang="ts">
  /** Actions on exactly the selected IDs (R12, R13). */
  import { untrack } from 'svelte';

  import { api, type DeleteResult } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { STAGES, stageLabel } from '../lib/stages';

  interface Props {
    ids: number[];
    ondeleted: (result: DeleteResult) => void;
    /** Queue cover letters for the selected rows (R23). */
    ongenerate?: (ids: number[]) => void;
  }

  let { ids, ondeleted, ongenerate }: Props = $props();

  let target = $state('applied');
  let busy = $state(false);
  let message = $state('');
  let problems = $state<string[]>([]);
  // Rows the move refused, which Reopen may take back (R11, D23).
  let refused = $state<number[]>([]);
  let refusedTarget = $state('');

  // A result belongs to the selection it was made for: when the selection
  // changes, forget it, so Reopen can only act on rows still selected (R12).
  let shownFor = '';
  $effect(() => {
    const key = [...ids].sort((a, b) => a - b).join(',');
    if (key !== shownFor) {
      shownFor = key;
      untrack(() => {
        refused = [];
        problems = [];
        message = '';
      });
    }
  });

  // Reopen never leads into Rejected or Withdrawn (D23).
  const canReopen = $derived(refused.length > 0 && refusedTarget !== 'rejected' && refusedTarget !== 'withdrawn');

  async function move(): Promise<void> {
    busy = true;
    message = '';
    problems = [];
    refused = [];
    try {
      const r = await api.setStage([...ids], target);
      message = `${r.changed.length} moved.`;
      problems = r.skipped.map((s) => s.reason);
      refused = r.skipped.map((s) => s.id);
      refusedTarget = target;
    } catch (err) {
      message = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function reopen(): Promise<void> {
    busy = true;
    try {
      const r = await api.reopen([...refused], refusedTarget);
      message = `${r.changed.length} reopened.`;
      problems = r.skipped.map((s) => s.reason);
      refused = [];
    } catch (err) {
      message = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function remove(): Promise<void> {
    busy = true;
    message = '';
    problems = [];
    refused = [];
    try {
      ondeleted(await api.deleteApplications([...ids]));
    } catch (err) {
      message = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="bulk" role="toolbar" aria-label="Selected applications">
  <b>{ids.length} selected</b>
  <label class="inline">
    <span>Move selected to</span>
    <select bind:value={target}>
      {#each STAGES as s (s.id)}
        <option value={s.id}>{s.label}</option>
      {/each}
    </select>
  </label>
  <button type="button" disabled={busy} onclick={move}>Move</button>
  {#if ongenerate}
    <button type="button" class="secondary" disabled={busy} onclick={() => ongenerate([...ids])}>Generate letters</button>
  {/if}
  <button type="button" class="danger" disabled={busy} onclick={remove}>Delete</button>
  {#if message}<span class="muted">{message}</span>{/if}
</div>
{#if problems.length > 0}
  <div class="problems">
    <ul>
      {#each problems as p, i (i)}<li>{p}</li>{/each}
    </ul>
    {#if canReopen}
      <button type="button" class="secondary" disabled={busy} onclick={reopen}>
        Reopen {refused.length} to {stageLabel(refusedTarget)}
      </button>
    {/if}
  </div>
{/if}

<style>
  .bulk {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
    padding: 0.5rem 0.75rem;
    margin-bottom: 0.5rem;
    border-radius: 8px;
    background: var(--accent-soft);
  }
  .inline {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .problems {
    margin: 0 0 0.5rem;
    color: var(--danger);
  }
  .problems ul {
    margin: 0 0 0.4rem;
  }
</style>
