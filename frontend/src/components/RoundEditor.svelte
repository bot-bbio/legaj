<script lang="ts">
  /**
   * Add or edit an interview round (R30). The time is entered as it appears
   * in the interview's own time zone; Go stores it as a UTC instant plus
   * that zone, so it stays right across travel and clock changes.
   */
  import { untrack } from 'svelte';

  import { api, type ApplicationRow, type Interviewer, type RoundInput, type RoundView } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { KINDS, userZone, zones } from '../lib/interviews';

  let {
    apps,
    round,
    onsaved,
    oncancel,
  }: {
    apps: readonly ApplicationRow[];
    round?: RoundView;
    onsaved: (r: RoundView) => void;
    oncancel: () => void;
  } = $props();

  // The parent re-creates the editor per round, so this copy is the draft.
  const initial = untrack(() => round);
  let form = $state<RoundInput>({
    appId: initial?.appId ?? 0,
    local: initial?.local ?? '',
    zone: initial?.zone ?? userZone(),
    durationMin: initial?.durationMin ?? 60,
    kind: initial?.kind ?? 'video',
    location: initial?.location ?? '',
    link: initial?.link ?? '',
    interviewers: initial ? initial.interviewers.map((p) => ({ ...p })) : [],
    prepNotes: initial?.prepNotes ?? '',
    postNotes: initial?.postNotes ?? '',
    outcome: initial?.outcome ?? '',
  });
  const zoneList = zones(initial?.zone);

  let busy = $state(false);
  let failure = $state('');

  function setPerson(i: number, field: keyof Interviewer, value: string): void {
    const p = form.interviewers[i];
    if (p) p[field] = value;
  }

  async function submit(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    busy = true;
    failure = '';
    try {
      const input: RoundInput = $state.snapshot(form);
      const saved = initial ? await api.updateRound(initial.id, input) : await api.addRound(input);
      onsaved(saved);
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<form autocomplete="off" class="card editor" aria-label="Interview round" onsubmit={submit}>
  <h2>{initial ? `Edit round: ${initial.company}` : 'Add an interview round'}</h2>

  {#if !initial}
    <label class="field">
      <span>Application</span>
      <select bind:value={form.appId}>
        <option value={0} disabled>Choose…</option>
        {#each apps as a (a.id)}
          <option value={a.id}>{a.company || 'Needs company'} · {a.role}</option>
        {/each}
      </select>
    </label>
  {/if}

  <div class="cols">
    <label class="field">
      <span>Date and time</span>
      <input autocomplete="off" type="datetime-local" bind:value={form.local} />
    </label>
    <label class="field">
      <span>Time zone</span>
      <select bind:value={form.zone}>
        {#each zoneList as z (z)}
          <option value={z}>{z}</option>
        {/each}
      </select>
    </label>
    <label class="field">
      <span>Minutes</span>
      <input autocomplete="off" type="number" min="5" max="720" step="5" bind:value={form.durationMin} />
    </label>
    <label class="field">
      <span>Type</span>
      <select bind:value={form.kind}>
        {#each KINDS as k (k.id)}
          <option value={k.id}>{k.label}</option>
        {/each}
      </select>
    </label>
  </div>

  <div class="cols">
    <label class="field"><span>Location</span><input autocomplete="off" type="text" bind:value={form.location} /></label>
    <label class="field">
      <span>Meeting link</span>
      <input autocomplete="off" type="url" placeholder="https://" bind:value={form.link} />
    </label>
  </div>

  <fieldset>
    <legend>Interviewers</legend>
    {#each form.interviewers as p, i (i)}
      <div class="cols person">
        <label class="field">
          <span>Name</span>
          <input autocomplete="off"
            type="text"
            aria-label="Interviewer {i + 1} name"
            value={p.name}
            oninput={(e) => setPerson(i, 'name', e.currentTarget.value)}
          />
        </label>
        <label class="field">
          <span>Title</span>
          <input autocomplete="off"
            type="text"
            aria-label="Interviewer {i + 1} title"
            value={p.title}
            oninput={(e) => setPerson(i, 'title', e.currentTarget.value)}
          />
        </label>
        <label class="field">
          <span>Email</span>
          <input autocomplete="off"
            type="email"
            aria-label="Interviewer {i + 1} email"
            value={p.email}
            oninput={(e) => setPerson(i, 'email', e.currentTarget.value)}
          />
        </label>
        <button
          type="button"
          class="secondary"
          onclick={() => (form.interviewers = form.interviewers.filter((_, j) => j !== i))}
        >
          Remove
        </button>
      </div>
    {/each}
    <button
      type="button"
      class="secondary"
      onclick={() => form.interviewers.push({ name: '', title: '', email: '' })}
    >
      Add interviewer
    </button>
  </fieldset>

  <label class="field"><span>Prep notes</span><textarea autocomplete="off" rows="3" bind:value={form.prepNotes}></textarea></label>
  <label class="field"><span>Notes after</span><textarea autocomplete="off" rows="3" bind:value={form.postNotes}></textarea></label>
  <label class="field"><span>Outcome</span><input autocomplete="off" type="text" bind:value={form.outcome} /></label>

  {#if failure}<p class="error" role="alert">{failure}</p>{/if}
  <div class="cols">
    <button type="submit" disabled={busy}>Save round</button>
    <button type="button" class="secondary" onclick={oncancel}>Cancel</button>
  </div>
</form>

<style>
  .editor {
    display: grid;
    gap: 0.6rem;
    max-width: 56rem;
    margin-bottom: 1rem;
  }
  .editor h2 {
    margin: 0;
  }
  fieldset {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
    display: grid;
    gap: 0.5rem;
    justify-items: start;
  }
  .person {
    align-items: flex-end;
  }
</style>
