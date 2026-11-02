<script lang="ts">
  import { api, type Profile, type ResumeImport } from '../../lib/api';
  import { errorMessage } from '../../lib/errors';

  let { ondraft, onnext }: { ondraft: (draft: Profile | null) => void; onnext: () => void } = $props();

  let busy = $state(false);
  let failure = $state('');
  let last = $state<ResumeImport | null>(null);

  async function run(call: () => Promise<ResumeImport>): Promise<void> {
    busy = true;
    failure = '';
    try {
      const r = await call();
      if (r.cancelled) return;
      last = r;
      if (r.draft) {
        ondraft(r.draft);
        onnext();
      } else {
        failure = r.parseError;
      }
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const retry = (): Promise<void> => {
    const file = last?.file;
    return file ? run(() => api.parseResume(file.id)) : Promise.resolve();
  };
</script>

<h2>Add your resume</h2>
<p>
  Choose your resume as a PDF or Word (.docx) file. LeGaJ keeps the file exactly as it is and uses it when you apply;
  it never rewrites your resume. Gemini reads it once to fill in your profile, which you check next.
</p>

<p>
  <button type="button" disabled={busy} onclick={() => run(() => api.uploadResume())}>
    {busy ? 'Reading your resume…' : 'Choose resume file…'}
  </button>
</p>

{#if last && !last.draft}
  <p class="muted">Saved {last.file.fileName}.</p>
{/if}
{#if failure}
  <p class="error" role="alert">{failure}</p>
  {#if last}
    <p class="actions">
      <button type="button" class="secondary" disabled={busy} onclick={retry}>Try reading it again</button>
      <button type="button" class="secondary" onclick={() => (ondraft(null), onnext())}>Fill in my profile by hand</button>
    </p>
  {/if}
{/if}

<div class="actions">
  <button type="button" class="secondary" onclick={() => (ondraft(null), onnext())}>Skip for now</button>
</div>
