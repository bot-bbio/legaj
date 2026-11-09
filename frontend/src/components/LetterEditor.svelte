<script lang="ts">
  /**
   * Review a drafted cover letter (R22): edit any paragraph, pick one of the
   * three why-this-company options, regenerate a single paragraph, then
   * Approve to render the next PDF version. Edits are saved when a field
   * loses focus and again before Regenerate or Approve, one save at a time.
   */
  import { api, type DocumentVersion, type Draft, type DraftPart } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  let {
    appId,
    onapproved,
  }: {
    appId: number;
    onapproved?: (v: DocumentVersion) => void;
  } = $props();

  let draft = $state<Draft | null>(null);
  let loading = $state(true);
  let busy = $state(false);
  let failure = $state('');
  let approved = $state<DocumentVersion | null>(null);

  // Saves run one after another so an older save can never land last.
  let queue: Promise<unknown> = Promise.resolve();

  $effect(() => {
    const id = appId;
    let stale = false;
    loading = true;
    failure = '';
    approved = null;
    api
      .getDraft(id)
      .then((d) => {
        if (!stale) draft = d;
      })
      .catch((e: unknown) => {
        if (!stale) failure = errorMessage(e);
      })
      .finally(() => {
        if (!stale) loading = false;
      });
    return () => {
      stale = true;
    };
  });

  async function persist(): Promise<boolean> {
    if (!draft) return false;
    const snap: Draft = $state.snapshot(draft);
    const run = queue.then(() => api.saveDraft(appId, snap));
    queue = run.catch(() => undefined);
    try {
      await run;
      failure = '';
      return true;
    } catch (e) {
      failure = errorMessage(e);
      return false;
    }
  }

  async function chooseWhy(i: number): Promise<void> {
    if (!draft) return;
    draft.chosenWhy = i;
    await persist();
  }

  async function regenerate(part: DraftPart, index: number): Promise<void> {
    busy = true;
    try {
      if (!(await persist())) return;
      draft = await api.regenerateParagraph(appId, part, index);
    } catch (e) {
      failure = errorMessage(e);
    } finally {
      busy = false;
    }
  }

  async function approve(): Promise<void> {
    busy = true;
    try {
      if (!(await persist())) return;
      const v = await api.approveLetter(appId);
      approved = v;
      draft = null;
      onapproved?.(v);
    } catch (e) {
      failure = errorMessage(e);
    } finally {
      busy = false;
    }
  }
</script>

<section class="card letter" aria-label="Cover letter review">
  {#if loading}
    <p class="muted">Loading the draft…</p>
  {:else if approved}
    <p class="notice" role="status">Saved as version {approved.version}.</p>
  {:else if draft}
    <label class="field">
      <span>Greeting</span>
      <input autocomplete="off" type="text" bind:value={draft.salutation} onchange={persist} disabled={busy} />
    </label>

    <div class="para">
      <label class="field">
        <span>Opening</span>
        <textarea autocomplete="off" rows="3" bind:value={draft.opening} onchange={persist} disabled={busy}></textarea>
      </label>
      <button type="button" class="secondary" disabled={busy} onclick={() => regenerate('opening', 0)}>
        Regenerate opening
      </button>
    </div>

    <fieldset class="why">
      <legend id="why-legend-{appId}">Why this company: pick one</legend>
      <div role="radiogroup" aria-labelledby="why-legend-{appId}">
        {#each draft.whyOptions, i (i)}
          <div class="option" class:chosen={draft.chosenWhy === i}>
            <label class="pick">
              <input
                type="radio"
                name="why-{appId}"
                checked={draft.chosenWhy === i}
                disabled={busy}
                onchange={() => chooseWhy(i)}
              />
              <span>{draft.whyOptions[i]}</span>
            </label>
            <textarea autocomplete="off"
              rows="3"
              aria-label="Why option {i + 1}"
              bind:value={draft.whyOptions[i]}
              onchange={persist}
              disabled={busy}
            ></textarea>
            <button type="button" class="secondary" disabled={busy} onclick={() => regenerate('why', i)}>
              Regenerate why option {i + 1}
            </button>
          </div>
        {/each}
      </div>
    </fieldset>

    {#each draft.body, i (i)}
      <div class="para">
        <label class="field">
          <span>Body paragraph {i + 1}</span>
          <textarea autocomplete="off" rows="4" bind:value={draft.body[i]} onchange={persist} disabled={busy}></textarea>
        </label>
        <button type="button" class="secondary" disabled={busy} onclick={() => regenerate('body', i)}>
          Regenerate body paragraph {i + 1}
        </button>
      </div>
    {/each}

    <div class="para">
      <label class="field">
        <span>Closing</span>
        <textarea autocomplete="off" rows="3" bind:value={draft.closing} onchange={persist} disabled={busy}></textarea>
      </label>
      <button type="button" class="secondary" disabled={busy} onclick={() => regenerate('closing', 0)}>
        Regenerate closing
      </button>
    </div>

    <label class="field">
      <span>Sign-off</span>
      <input autocomplete="off" type="text" bind:value={draft.signOff} onchange={persist} disabled={busy} />
    </label>

    <div class="cols">
      <button type="button" disabled={busy} onclick={approve}>Approve letter</button>
    </div>
  {/if}

  {#if failure}<p class="error" role="alert">{failure}</p>{/if}
</section>

<style>
  .letter {
    display: grid;
    gap: 0.75rem;
    max-width: 56rem;
  }
  .para {
    display: grid;
    gap: 0.35rem;
    justify-items: start;
  }
  .para .field {
    width: 100%;
  }
  .why {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.75rem;
  }
  .option {
    display: grid;
    gap: 0.35rem;
    justify-items: start;
    padding: 0.5rem;
    border-radius: 6px;
  }
  .option textarea {
    width: 100%;
  }
  .option.chosen {
    background: var(--accent-soft);
  }
  .pick {
    display: flex;
    gap: 0.5rem;
    align-items: flex-start;
  }
  .pick span {
    font-size: 0.85rem;
    color: var(--muted);
  }
  .notice {
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
</style>
