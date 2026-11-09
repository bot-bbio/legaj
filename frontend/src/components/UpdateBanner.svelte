<script lang="ts">
  /**
   * Notify-only update banner (R40). Asks once per launch; Download opens
   * the GitHub release page in the browser. Nothing is downloaded or
   * installed by LeGaJ, and with the Settings toggle off Go makes no request.
   */
  import { onMount } from 'svelte';

  import { api, type UpdateResult } from '../lib/api';

  let result = $state<UpdateResult | null>(null);
  let dismissed = $state(false);

  onMount(() => {
    void (async () => {
      try {
        result = await api.checkForUpdate();
      } catch {
        result = null; // a failed check is the same as "no update"
      }
    })();
  });

  const show = $derived(!dismissed && result !== null && result.available);
</script>

{#if show && result}
  <div class="banner" role="status">
    <span>LeGaJ {result.latest} is available.</span>
    <button type="button" onclick={() => result && void api.openURL(result.url)}>Download</button>
    <button type="button" class="secondary" onclick={() => (dismissed = true)}>Not now</button>
  </div>
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
