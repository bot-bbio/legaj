<script lang="ts">
  /**
   * Interviews (R30, R31): Upcoming and Past rounds across all
   * applications. Each round shows its time in its own zone, and in the
   * user's when they differ; Add to calendar exports an .ics file.
   */
  import RoundEditor from '../components/RoundEditor.svelte';
  import { api, type RoundView } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { kindLabel, userZone, wallClock } from '../lib/interviews';
  import { applicationsStore } from '../lib/stores/applications';
  import { roundsStore } from '../lib/stores/rounds';

  let adding = $state(false);
  let editing = $state<number | null>(null);
  let confirmDelete = $state<number | null>(null);
  let failure = $state('');

  const apps = $derived($applicationsStore.status === 'ready' ? $applicationsStore.data : []);
  const lists = $derived($roundsStore.status === 'ready' ? $roundsStore.data : { upcoming: [], past: [] });
  const myZone = userZone();

  const yourTime = (iso: string): string =>
    new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });

  async function saved(): Promise<void> {
    adding = false;
    editing = null;
    await roundsStore.refresh();
  }

  async function run(fn: () => Promise<void>): Promise<void> {
    failure = '';
    try {
      await fn();
    } catch (err) {
      failure = errorMessage(err);
    }
  }

  const remove = (r: RoundView): Promise<void> =>
    run(async () => {
      await api.deleteRound(r.id);
      confirmDelete = null;
      await roundsStore.refresh();
    });
</script>

<div class="title">
  <h1>Interviews</h1>
  {#if !adding}
    <button type="button" onclick={() => ((adding = true), (editing = null))}>Add round</button>
  {/if}
</div>

{#if adding}
  <RoundEditor {apps} onsaved={saved} oncancel={() => (adding = false)} />
{/if}
{#if failure}<p class="error" role="alert">{failure}</p>{/if}

{#snippet card(r: RoundView)}
  <article class="card round" aria-label="{r.company || 'Untitled'} {kindLabel(r.kind)} interview">
    {#if editing === r.id}
      <RoundEditor {apps} round={r} onsaved={saved} oncancel={() => (editing = null)} />
    {:else}
      <div class="head">
        <p><strong>{r.company || 'Untitled application'}</strong> <span class="muted">{r.role}</span></p>
        <p class="badge">{kindLabel(r.kind)} · {r.durationMin} min</p>
      </div>
      <p>
        {wallClock(r.local)} <span class="muted">({r.zone})</span>
        {#if r.zone !== myZone}<br /><span class="muted">Your time: {yourTime(r.startsAt)}</span>{/if}
      </p>
      {#if r.location}<p>Location: {r.location}</p>{/if}
      {#if r.interviewers.length > 0}
        <p>With {r.interviewers.map((p) => (p.title ? `${p.name} (${p.title})` : p.name)).join(', ')}</p>
      {/if}
      {#if r.outcome}<p>Outcome: <strong>{r.outcome}</strong></p>{/if}
      {#if r.prepNotes}<p class="notes"><span class="muted">Prep:</span> {r.prepNotes}</p>{/if}
      {#if r.postNotes}<p class="notes"><span class="muted">After:</span> {r.postNotes}</p>{/if}
      <div class="actions">
        {#if r.link}
          <button type="button" class="secondary" onclick={() => run(() => api.openURL(r.link))}>Open meeting link</button>
        {/if}
        <button type="button" class="secondary" onclick={() => run(() => api.addToCalendar(r.id))}>Add to calendar</button>
        <button type="button" class="secondary" onclick={() => ((editing = r.id), (adding = false))}>Edit</button>
        {#if confirmDelete === r.id}
          <span>Delete this round?</span>
          <button type="button" class="danger" onclick={() => remove(r)}>Delete</button>
          <button type="button" class="secondary" onclick={() => (confirmDelete = null)}>Keep</button>
        {:else}
          <button type="button" class="secondary" onclick={() => (confirmDelete = r.id)}>Delete…</button>
        {/if}
      </div>
    {/if}
  </article>
{/snippet}

{#if $roundsStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $roundsStore.status === 'error'}
  <p class="error">{$roundsStore.message}</p>
{:else}
  <section aria-label="Upcoming">
    <h2>Upcoming</h2>
    {#each lists.upcoming as r (r.id)}
      {@render card(r)}
    {:else}
      <p class="muted">No upcoming interviews. Add a round when one is scheduled.</p>
    {/each}
  </section>
  <section aria-label="Past">
    <h2>Past</h2>
    {#each lists.past as r (r.id)}
      {@render card(r)}
    {:else}
      <p class="muted">No past interviews yet.</p>
    {/each}
  </section>
{/if}

<style>
  .title {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .round {
    display: grid;
    gap: 0.35rem;
    max-width: 56rem;
    margin-bottom: 0.75rem;
  }
  .round p {
    margin: 0;
  }
  .head {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
  }
  .badge {
    padding: 0.1rem 0.5rem;
    border-radius: 999px;
    background: var(--accent-soft);
    font-size: 0.85em;
  }
  .notes {
    white-space: pre-wrap;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
  }
</style>
