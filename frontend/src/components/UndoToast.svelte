<script lang="ts">
  /**
   * "Deleted N applications. Undo" (R13). The backend purges 10 s after the
   * delete commits; the toast appears a little later (one round trip), so
   * it closes after 9.5 s and never offers an Undo that can no longer work.
   * A note about a partial undo stays until the user closes it.
   */
  import { api, type DeleteResult } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  const OFFER_MS = 9_500; // under pipeline.UndoWindow (10 s) in Go

  let { result, onclose }: { result: DeleteResult; onclose: () => void } = $props();

  let busy = $state(false);
  let note = $state('');

  $effect(() => {
    if (note) return; // a note waits for the user, not a timer
    const timer = setTimeout(onclose, OFFER_MS);
    return () => clearTimeout(timer);
  });

  async function undo(): Promise<void> {
    busy = true;
    try {
      const r = await api.undoDelete(result.undoId);
      if (r.skipped.length > 0) {
        const n = r.skipped.length;
        note =
          `${n} couldn’t be restored because you added the same job again; ` +
          `${n === 1 ? 'its' : 'their'} letters, interview rounds and history moved to the job you added again.`;
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
    display: flex;
    align-items: center;
    gap: 1rem;
    max-width: 40rem;
    padding: 0.7rem 1rem;
    border-radius: 8px;
    background: var(--text);
    color: var(--bg);
    box-shadow: 0 6px 24px rgb(0 0 0 / 0.25);
  }
  .toast .secondary {
    color: var(--bg);
    border-color: var(--bg);
  }
</style>
