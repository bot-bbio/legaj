<script lang="ts">
  /** Actions on exactly the selected IDs (R12, R13). */
  import { api, type DeleteResult } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { STAGES, stageLabel } from '../lib/stages';

  interface Props {
    ids: number[];
    ondeleted: (result: DeleteResult) => void;
  }

  let { ids, ondeleted }: Props = $props();

  let target = $state('applied');
  let busy = $state(false);
  let message = $state('');
  let problems = $state<string[]>([]);
  // Rows the move refused, which Reopen may take back (R11, D23).
  let refused = $state<number[]>([]);
  let refusedTarget = $state('');

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
  <button type="button" class="danger" disabled={busy} onclick={remove}>Delete</button>
  {#if message}<span class="muted">{message}</span>{/if}
</div>
{#if problems.length > 0}
  <div class="problems">
    <ul>
      {#each problems as p, i (i)}<li>{p}</li>{/each}
    </ul>
    {#if refused.length > 0}
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
