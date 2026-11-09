<script lang="ts">
  /**
   * First-run wizard (R7): key, resume, profile review, clipper. Every step
   * can be skipped; skipped steps stay on the setup checklist. Finish lands
   * on the Profile page, which shows the saved data at once (R3, U1).
   */
  import { api, type Profile, type StepID } from '../../lib/api';
  import { errorMessage } from '../../lib/errors';
  import { closeWizard, onboardingStore } from '../../lib/stores/onboarding';
  import { navigate } from '../../lib/stores/route';
  import ClipperStep from './ClipperStep.svelte';
  import KeyStep from './KeyStep.svelte';
  import ResumeStep from './ResumeStep.svelte';
  import ReviewStep from './ReviewStep.svelte';

  let { start = 'key' }: { start?: StepID } = $props();

  const steps: { id: StepID; label: string }[] = [
    { id: 'key', label: 'Gemini key' },
    { id: 'resume', label: 'Resume' },
    { id: 'profile', label: 'Profile' },
    { id: 'clipper', label: 'Clipper' },
  ];

  let index = $state(Math.max(0, steps.findIndex((s) => s.id === start)));
  let draft = $state<Profile | null>(null);
  let finishing = $state(false);
  let failure = $state('');

  const current = $derived(steps[index]?.id ?? 'key');
  const keySaved = $derived(
    $onboardingStore.status === 'ready' && $onboardingStore.data.steps.some((s) => s.id === 'key' && s.done),
  );

  function next(): void {
    if (index < steps.length - 1) index += 1;
  }

  async function finish(): Promise<void> {
    finishing = true;
    failure = '';
    try {
      await api.finishOnboarding();
      closeWizard();
      navigate('profile');
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      finishing = false;
    }
  }
</script>

<div class="wizard">
  <header>
    <h1>Set up LeGaJ</h1>
    <ol class="progress">
      {#each steps as s, i (s.id)}
        <li aria-current={i === index ? 'step' : undefined} class:done={i < index}>{s.label}</li>
      {/each}
    </ol>
  </header>

  <section class="card">
    {#if current === 'key'}
      <KeyStep alreadySaved={keySaved} onnext={next} />
    {:else if current === 'resume'}
      <ResumeStep ondraft={(d) => (draft = d)} onnext={next} />
    {:else if current === 'profile'}
      <ReviewStep {draft} onnext={next} />
    {:else}
      <ClipperStep onfinish={finish} busy={finishing} />
    {/if}
    {#if failure}<p class="error" role="alert">{failure}</p>{/if}
  </section>

  {#if index > 0}
    <p><button type="button" class="secondary" onclick={() => (index -= 1)}>Back</button></p>
  {/if}
</div>

<style>
  .wizard {
    max-width: 46rem;
    margin: 2rem auto;
    padding: 0 1.5rem;
  }
  .progress {
    display: flex;
    gap: 1.5rem;
    padding: 0;
    list-style: none;
    color: var(--muted);
  }
  .progress li[aria-current='step'] {
    color: var(--accent);
    font-weight: 600;
  }
  .progress li.done {
    color: var(--text);
  }
  .wizard :global(.actions) {
    display: flex;
    gap: 0.75rem;
    margin-top: 1.25rem;
  }
</style>
