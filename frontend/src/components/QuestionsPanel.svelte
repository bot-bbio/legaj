<script lang="ts">
  /**
   * Questions to ask the interviewer (R32): generated only on request, only
   * for applications in Interviewing, and saved on the round. The prompt
   * leaves out anything the job description already answers (D9).
   */
  import { api, type RoundView } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  let { round }: { round: RoundView } = $props();

  // The saved questions; a new set shows at once, and a refreshed round
  // (from the store) replaces it.
  let questions = $derived<string[]>([...round.questions]);

  let busy = $state(false);
  let failure = $state('');
  let copied = $state(false);

  const allowed = $derived(round.stage === 'interviewing');

  async function suggest(): Promise<void> {
    busy = true;
    failure = '';
    copied = false;
    try {
      const r = await api.suggestQuestions(round.id);
      questions = r.questions;
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function copyAll(): Promise<void> {
    failure = '';
    try {
      await api.copyText(questions.map((q) => `- ${q}`).join('\n'));
      copied = true;
    } catch (err) {
      failure = errorMessage(err);
    }
  }
</script>

<section class="questions" aria-label="Questions to ask">
  <div class="head">
    <h3>Questions to ask</h3>
    <button type="button" class="secondary" disabled={!allowed || busy} onclick={suggest}>
      {questions.length > 0 ? 'Suggest questions again' : 'Suggest questions'}
    </button>
    {#if questions.length > 0}
      <button type="button" class="secondary" onclick={copyAll}>Copy all</button>
    {/if}
  </div>
  {#if !allowed}
    <p class="muted">Only for applications in Interviewing.</p>
  {/if}
  {#if busy}<p class="muted">Thinking of questions…</p>{/if}
  {#if failure}<p class="error" role="alert">{failure}</p>{/if}
  {#if copied}<p class="muted" role="status">Copied.</p>{/if}
  {#if questions.length > 0}
    <ol>
      {#each questions as q, i (i)}<li>{q}</li>{/each}
    </ol>
  {/if}
</section>

<style>
  .questions {
    border-top: 1px solid var(--border);
    padding-top: 0.5rem;
  }
  .head {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  h3 {
    margin: 0 auto 0 0;
    font-size: 1rem;
  }
  ol {
    margin: 0.4rem 0 0;
    padding-left: 1.4rem;
  }
</style>
