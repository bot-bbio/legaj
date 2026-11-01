<script lang="ts">
  /**
   * Renders the app only once the database is ready. Otherwise it shows the
   * choice from item 4: restore a verified backup, or quit. LeGaJ never
   * silently starts over with an empty database (R2).
   */
  import { onMount, type Snippet } from 'svelte';

  import { api, type BackupView, type StartupView } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  let { children }: { children: Snippet } = $props();

  let view = $state<StartupView | null>(null);
  let failure = $state('');
  let busy = $state(false);

  onMount(() => {
    api
      .startup()
      .then((v) => (view = v))
      .catch((e: unknown) => (failure = errorMessage(e)));
  });

  async function restore(b: BackupView): Promise<void> {
    busy = true;
    failure = '';
    try {
      view = await api.restoreBackup(b.name);
    } catch (e) {
      failure = errorMessage(e);
    } finally {
      busy = false;
    }
  }

  const when = (iso: string): string =>
    new Date(iso).toLocaleString(undefined, { dateStyle: 'full', timeStyle: 'short' });
  const size = (bytes: number): string => `${Math.max(1, Math.round(bytes / 1024))} KB`;
</script>

{#if view?.kind === 'ready'}
  {@render children()}
{:else if view === null && failure === ''}
  <p class="center muted">Starting LeGaJ…</p>
{:else}
  <section class="panel" aria-live="polite">
    {#if view?.kind === 'corrupt'}
      <h1>Your LeGaJ data file is damaged</h1>
      <p>
        LeGaJ moved it aside as <code>{view.movedAside}</code> and did <strong>not</strong> create an empty one. Restore
        a backup to carry on where it was taken, or quit.
      </p>
    {:else if view?.kind === 'migration_failed'}
      <h1>LeGaJ couldn’t upgrade your data</h1>
      <p>Nothing was changed. Restore a backup taken before the upgrade, or quit and report the problem.</p>
    {:else}
      <h1>LeGaJ couldn’t start</h1>
      <p>{view?.message || failure}</p>
    {/if}

    {#if view && view.kind !== 'error'}
      {#if view.backups.length > 0}
        <ul class="backups">
          {#each view.backups as b (b.name)}
            <li>
              <span>{when(b.takenAt)} <span class="muted">· {size(b.sizeBytes)}</span></span>
              <button type="button" disabled={busy} onclick={() => restore(b)}>Restore this backup</button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted">No usable backup was found.</p>
      {/if}
    {/if}

    {#if failure && view && view.kind !== 'error'}
      <p class="error">{failure}</p>
    {/if}
    <p><button type="button" class="secondary" onclick={() => api.quit()}>Quit LeGaJ</button></p>
  </section>
{/if}

<style>
  .center {
    display: grid;
    place-items: center;
    height: 100vh;
  }
  .muted {
    color: var(--muted);
  }
  .panel {
    max-width: 40rem;
    margin: 4rem auto;
    padding: 2rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
  }
  .backups {
    list-style: none;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .backups li {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 6px;
  }
  button {
    font: inherit;
    padding: 0.45rem 0.9rem;
    border-radius: 6px;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: #fff;
    cursor: pointer;
  }
  button.secondary {
    background: transparent;
    color: var(--text);
    border-color: var(--border);
  }
  button:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .error {
    color: var(--danger);
  }
</style>
