<script lang="ts">
  /** Skipped setup steps, resumable at any time (R7). */
  import type { StepID } from '../lib/api';
  import { isStepID, onboardingStore, openWizard } from '../lib/stores/onboarding';

  const labels: Record<StepID, string> = {
    key: 'Add your Gemini API key',
    resume: 'Upload your resume',
    profile: 'Check your profile',
    clipper: 'Install the clipper',
  };

  const pending = $derived(
    $onboardingStore.status === 'ready'
      ? $onboardingStore.data.steps.filter((s) => !s.done).flatMap((s) => (isStepID(s.id) ? [s.id] : []))
      : [],
  );
</script>

{#if pending.length > 0}
  <aside class="checklist" aria-label="Finish setting up">
    <strong>Finish setting up LeGaJ</strong>
    <ul>
      {#each pending as id (id)}
        <li><button type="button" class="link" onclick={() => openWizard(id)}>{labels[id]}</button></li>
      {/each}
    </ul>
  </aside>
{/if}

<style>
  .checklist {
    margin-bottom: 1.25rem;
    padding: 0.75rem 1rem;
    border: 1px solid var(--border);
    border-left: 4px solid var(--accent);
    border-radius: 8px;
    background: var(--surface);
  }
  ul {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 1.25rem;
    margin: 0.4rem 0 0;
    padding: 0;
    list-style: none;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    text-decoration: underline;
    cursor: pointer;
  }
</style>
