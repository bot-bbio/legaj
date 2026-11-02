<script lang="ts">
  /**
   * The resume LeGaJ uses (R10, gate C5). Uploading a new version only stores
   * the file: the profile isn't touched, and applications already marked
   * Applied keep the version they were sent with.
   */
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { resumeStore } from '../lib/stores/resume';

  let busy = $state(false);
  let note = $state('');
  let failure = $state('');

  async function upload(): Promise<void> {
    busy = true;
    note = '';
    failure = '';
    try {
      const r = await api.uploadNewResume();
      if (!r.cancelled) {
        note = `Saved ${r.file.fileName}. New applications will use it; applications already marked Applied keep the version they were sent with.`;
        void resumeStore.refresh();
      }
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function open(): Promise<void> {
    failure = '';
    try {
      await api.openCurrentResume();
    } catch (err) {
      failure = errorMessage(err);
    }
  }

  const when = (iso: string): string => new Date(iso).toLocaleDateString(undefined, { dateStyle: 'medium' });
</script>

<section class="card" aria-label="Resume">
  <h2>Resume</h2>
  {#if $resumeStore.status === 'ready' && $resumeStore.data.id > 0}
    <p class="cols">
      <span><b>{$resumeStore.data.fileName}</b> <span class="muted">· uploaded {when($resumeStore.data.uploadedAt)}</span></span>
      <button type="button" class="secondary" onclick={open}>Open</button>
      <button type="button" disabled={busy} onclick={upload}>Upload a new version</button>
    </p>
  {:else if $resumeStore.status === 'error'}
    <p class="error">{$resumeStore.message}</p>
  {:else}
    <p class="cols">
      <span class="muted">No resume uploaded yet.</span>
      <button type="button" disabled={busy} onclick={upload}>Upload your resume</button>
    </p>
  {/if}
  {#if note}<p class="muted" role="status">{note}</p>{/if}
  {#if failure}<p class="error" role="alert">{failure}</p>{/if}
</section>
