<script lang="ts">
  /** Version, license and the author credit (R41), at the foot of Settings. */
  import { onMount } from 'svelte';

  import { api } from '../lib/api';

  const CREDIT_URL = 'https://linkedin.com/in/alvvays';

  let version = $state('');
  onMount(() => {
    void (async () => {
      try {
        version = await api.version();
      } catch {
        version = '';
      }
    })();
  });
</script>

<footer class="about">
  <p>
    LeGaJ {version} · Free and open source under the MIT License · Made by
    <button type="button" class="link" onclick={() => void api.openURL(CREDIT_URL)}>Roberto Montero</button>
  </p>
</footer>

<style>
  .about {
    margin-top: 2rem;
    color: var(--muted);
    font-size: 0.9em;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    text-decoration: underline;
    cursor: pointer;
    font: inherit;
  }
</style>
