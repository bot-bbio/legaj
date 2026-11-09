<script lang="ts">
  /**
   * The answer bank (R25): every approved answer, kept even after its
   * application is deleted. A Pipeline sub-view, never a nav section (R1).
   */
  import { api } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { fold } from '../lib/selection';
  import { answerBankStore } from '../lib/stores/answers';

  let query = $state('');
  let message = $state('');
  let failure = $state('');

  const all = $derived($answerBankStore.status === 'ready' ? $answerBankStore.data : []);
  const shown = $derived(
    query.trim() === ''
      ? all
      : all.filter((a) => fold(`${a.question} ${a.answer}`).includes(fold(query.trim()))),
  );

  async function run(fn: () => Promise<void>, done: string): Promise<void> {
    message = '';
    failure = '';
    try {
      await fn();
      message = done;
    } catch (err) {
      failure = errorMessage(err);
    }
  }
</script>

<div class="bank">
  <label class="field">
    <span>Search the bank</span>
    <input autocomplete="off" type="search" placeholder="Question or answer" bind:value={query} />
  </label>
  {#if message}<p class="notice" role="status">{message}</p>{/if}
  {#if failure}<p class="error" role="alert">{failure}</p>{/if}

  {#if $answerBankStore.status === 'loading'}
    <p class="muted">Loading…</p>
  {:else if $answerBankStore.status === 'error'}
    <p class="error">{$answerBankStore.message}</p>
  {:else if all.length === 0}
    <p class="muted">No banked answers yet. Approve an answer in a row’s Answers panel and it’s saved here.</p>
  {:else if shown.length === 0}
    <p class="muted">No banked answer matches “{query}”.</p>
  {:else}
    {#each shown as a (a.id)}
      <article class="card" aria-label={a.question}>
        <h3>{a.question}</h3>
        <p>{a.answer}</p>
        <div class="actions">
          <button type="button" class="secondary" onclick={() => run(() => api.copyText(a.answer), 'Copied.')}>Copy</button>
          <button type="button" class="secondary" onclick={() => run(() => api.removeFromBank(a.id), 'Removed from the bank.')}>
            Remove from bank
          </button>
        </div>
      </article>
    {/each}
  {/if}
</div>

<style>
  .bank {
    display: grid;
    gap: 0.75rem;
    max-width: 56rem;
  }
  h3 {
    margin: 0 0 0.25rem;
    font-size: 1rem;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
  }
  .notice {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
</style>
