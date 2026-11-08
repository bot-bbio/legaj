<script lang="ts">
  /**
   * Add a job by hand (R15): the fallback when the clipper can't be used.
   * If the posting is already in the pipeline, the existing row is offered
   * instead of a duplicate.
   */
  import { api, type PasteInput } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  let { onclose, onshow }: { onclose: () => void; onshow: (id: number) => void } = $props();

  let form = $state<PasteInput>({ url: '', company: '', role: '', location: '', description: '' });
  let busy = $state(false);
  let failure = $state('');
  let duplicateOf = $state(0);

  async function submit(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    busy = true;
    failure = '';
    duplicateOf = 0;
    try {
      const r = await api.pasteJob({ ...form });
      if (r.created) onclose();
      else duplicateOf = r.duplicateOf;
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }
</script>

<section class="card paste" aria-label="Paste job">
  <h2>Paste a job</h2>
  <form autocomplete="off" onsubmit={submit}>
    <label class="field"><span>Job URL</span><input autocomplete="off" type="url" placeholder="https://" bind:value={form.url} /></label>
    <div class="cols">
      <label class="field"><span>Company</span><input autocomplete="off" type="text" bind:value={form.company} /></label>
      <label class="field"><span>Role</span><input autocomplete="off" type="text" bind:value={form.role} /></label>
      <label class="field"><span>Location</span><input autocomplete="off" type="text" bind:value={form.location} /></label>
    </div>
    <label class="field">
      <span>Job description (paste the whole posting; letters and answers are based on it)</span>
      <textarea autocomplete="off" rows="8" bind:value={form.description}></textarea>
    </label>

    {#if duplicateOf}
      <p class="notice">
        This job is already in your pipeline.
        <button type="button" class="secondary" onclick={() => onshow(duplicateOf)}>Show it</button>
      </p>
    {/if}
    {#if failure}<p class="error" role="alert">{failure}</p>{/if}

    <div class="cols">
      <button type="submit" disabled={busy}>Add to pipeline</button>
      <button type="button" class="secondary" onclick={onclose}>Cancel</button>
    </div>
  </form>
</section>

<style>
  .paste {
    max-width: 56rem;
  }
  .notice {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
</style>
