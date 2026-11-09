<script lang="ts">
  /**
   * Says when the clipper needs attention (R18): another program took its
   * port, so the bookmark in the browser points at nothing, or it couldn't
   * start at all. Everything else about the clipper lives in Settings.
   */
  import { clipStore } from '../lib/stores/clip';
  import { navigate } from '../lib/stores/route';

  const status = $derived($clipStore.status === 'ready' ? $clipStore.data : null);
</script>

{#if status?.needsReinstall}
  <div class="banner" role="status">
    <span>Your clipper bookmark no longer reaches LeGaJ. Reinstall it, then clip a job to clear this message.</span>
    <button type="button" onclick={() => navigate('settings')}>Reinstall bookmarklet</button>
  </div>
{:else if status && !status.running && status.message}
  <div class="banner" role="alert"><span>{status.message}</span></div>
{/if}

<style>
  .banner {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    background: var(--accent-soft);
    border-bottom: 1px solid var(--border);
  }
  .banner span {
    margin-right: auto;
  }
</style>
