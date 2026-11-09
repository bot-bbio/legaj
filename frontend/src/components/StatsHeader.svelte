<script lang="ts">
  /** Per-stage counts, the newest 5 applications and the next interview (R14). */
  import { STAGES } from '../lib/stages';
  import { statsStore } from '../lib/stores/stats';

  const when = (iso: string): string =>
    new Date(iso).toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
</script>

{#if $statsStore.status === 'ready'}
  {@const st = $statsStore.data}
  <section class="stats" aria-label="Pipeline summary">
    <ul class="counts">
      {#each STAGES as s (s.id)}
        <li><span class="muted">{s.label}</span> <b>{st.counts[s.id] ?? 0}</b></li>
      {/each}
    </ul>
    <div class="cols2">
      <div>
        <h2>Recently added</h2>
        {#if st.recent.length === 0}
          <p class="muted">Nothing yet.</p>
        {:else}
          <ul aria-label="Recently added">
            {#each st.recent as r (r.id)}
              <li>{r.company || 'Needs company'}<span class="muted"> · {r.role}</span></li>
            {/each}
          </ul>
        {/if}
      </div>
      <div>
        <h2>Next interview</h2>
        {#if st.nextInterview}
          <p>
            <b>{st.nextInterview.company}</b>, {st.nextInterview.role}<br />
            <span class="muted">{when(st.nextInterview.startsAt)}</span>
          </p>
        {:else}
          <p class="muted">No interviews scheduled.</p>
        {/if}
      </div>
    </div>
  </section>
{/if}

<style>
  .stats {
    margin-bottom: 1rem;
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--surface);
  }
  .counts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem 1.25rem;
    margin: 0 0 0.75rem;
    padding: 0;
    list-style: none;
  }
  .cols2 {
    display: grid;
    grid-template-columns: 2fr 1fr;
    gap: 1.5rem;
  }
  h2 {
    margin: 0 0 0.35rem;
    font-size: 0.95rem;
  }
  ul[aria-label] {
    margin: 0;
    padding-left: 1.1rem;
  }
  p {
    margin: 0;
  }
</style>
