<script lang="ts">
  /**
   * "Deleted N applications. Undo" for the backend's 10-second undo window
   * (R13). The toast closes when the window ends, since the rows are then
   * purged.
   */
  import { api, type DeleteResult } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  const UNDO_WINDOW_MS = 10_000; // matches pipeline.UndoWindow in Go

  let { result, onclose }: { result: DeleteResult; onclose: () => void } = $props();

  let busy = $state(false);
  let note = $state('');

  $effect(() => {
    const timer = setTimeout(onclose, UNDO_WINDOW_MS);
    return () => clearTimeout(timer);
  });

  async function undo(): Promise<void> {
    busy = true;
    try {
      const r = await api.undoDelete(result.undoId);
      if (r.skipped.length > 0) {
        note = `${r.skipped.length} couldn’t be restored because the same job was added again.`;
        return;
      }
      onclose();
    } catch (err) {
      note = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const noun = $derived(result.ids.length === 1 ? 'application' : 'applications');
</script>

<div class="toast" role="status">
  {#if note}
    <span>{note}</span>
    <button type="button" class="secondary" onclick={onclose}>Close</button>
  {:else}
    <span>Deleted {result.ids.length} {noun}.</span>
    <button type="button" disabled={busy} onclick={undo}>Undo</button>
  {/if}
</div>

<style>
  .toast {
    position: fixed;
    left: 50%;
    bottom: 1.5rem;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.7rem 1rem;
    border-radius: 8px;
    background: var(--text);
    color: var(--bg);
    box-shadow: 0 6px 24px rgb(0 0 0 / 0.25);
  }
</style>
