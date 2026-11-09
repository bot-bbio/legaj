<script lang="ts">
  /**
   * Form answers for one application (R25), in Pipeline's side panel.
   * Typing a question first shows banked answers to similar questions;
   * Generate drafts three options; an option (edited or not) is approved
   * into the answer bank. Every answer has a Copy button for pasting into
   * the application form.
   */
  import { onDestroy, untrack } from 'svelte';

  import { api, type Answer, type ApplicationRow, type Suggestion } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { entityStore } from '../lib/stores/entity';

  let { row, onclose }: { row: ApplicationRow; onclose: () => void } = $props();

  // The parent re-creates this panel per application ({#key}), so reading
  // the ID once is intended: the store below belongs to this one row.
  const appId = untrack(() => row.id);
  const answers = entityStore(['answer'], () => api.listAnswers(appId));
  const list = $derived($answers.status === 'ready' ? $answers.data : []);

  let question = $state('');
  let busy = $state(false);
  let message = $state('');
  let failure = $state('');
  let suggestions = $state<Suggestion[]>([]);
  // Typed edits of draft options, by "<answer id>:<updated>:<option index>".
  // Regenerating a question updates the same answer, so its new options
  // (a new updated time) never show an edit typed over the old ones.
  let edits = $state<Record<string, string>>({});

  const name = $derived(row.company || row.role || 'this application');

  // Banked answers to similar questions are offered before generating.
  let timer: ReturnType<typeof setTimeout> | undefined;
  let asked = 0;
  function questionChanged(): void {
    clearTimeout(timer);
    const q = question.trim();
    if (q.length < 3) {
      suggestions = [];
      return;
    }
    timer = setTimeout(() => {
      const mine = ++asked;
      api
        .suggestAnswers(q)
        .then((s) => {
          if (mine === asked) suggestions = s;
        })
        .catch(() => {
          if (mine === asked) suggestions = [];
        });
    }, 250);
  }
  onDestroy(() => clearTimeout(timer));

  async function run(fn: () => Promise<void>): Promise<void> {
    busy = true;
    failure = '';
    message = '';
    try {
      await fn();
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const generate = (): Promise<void> =>
    run(async () => {
      await api.generateAnswers(appId, question.trim());
      question = '';
      suggestions = [];
      await answers.refresh();
    });

  const copy = (text: string): Promise<void> =>
    run(async () => {
      await api.copyText(text);
      message = 'Copied.';
    });

  const key = (a: Answer, i: number): string => `${a.id}:${a.updated}:${i}`;
  const optionText = (a: Answer, i: number): string => edits[key(a, i)] ?? a.options[i] ?? '';

  const approve = (a: Answer, i: number): Promise<void> =>
    run(async () => {
      await api.approveAnswer(a.id, optionText(a, i));
      message = 'Approved and saved to your answer bank.';
      await answers.refresh();
    });

  const remove = (a: Answer): Promise<void> =>
    run(async () => {
      await api.deleteAnswer(a.id);
      await answers.refresh();
    });
</script>

<div class="card answers" role="complementary" aria-label="Answers for {name}">
  <div class="head">
    <h2>Answers: {name}</h2>
    <button type="button" class="secondary" onclick={onclose}>Close</button>
  </div>

  <label class="field">
    <span>Question (leave empty for “Why this company?”)</span>
    <textarea autocomplete="off" rows="2" bind:value={question} oninput={questionChanged} disabled={busy}></textarea>
  </label>

  {#if suggestions.length > 0}
    <section class="suggestions" aria-label="From your answer bank">
      <h3>From your answer bank</h3>
      {#each suggestions as s (s.answer.id)}
        <div class="banked">
          <p class="muted">{s.answer.question}</p>
          <p>{s.answer.answer}</p>
          <button type="button" class="secondary" disabled={busy} onclick={() => copy(s.answer.answer)}>Copy</button>
        </div>
      {/each}
    </section>
  {/if}

  <div>
    <button type="button" disabled={busy} onclick={generate}>Generate answers</button>
  </div>

  {#if message}<p class="notice" role="status">{message}</p>{/if}
  {#if failure}<p class="error" role="alert">{failure}</p>{/if}

  {#if $answers.status === 'loading'}
    <p class="muted">Loading…</p>
  {:else if $answers.status === 'error'}
    <p class="error">{$answers.message}</p>
  {:else if list.length === 0}
    <p class="muted">No answers yet for this application.</p>
  {:else}
    {#each list as a (a.id)}
      <article aria-label={a.question}>
        <h3>{a.question}</h3>
        {#if a.status === 'approved'}
          <p>{a.answer}</p>
          <div class="actions">
            <button type="button" class="secondary" disabled={busy} onclick={() => copy(a.answer)}>Copy answer</button>
            {#if a.inBank}<span class="badge">In answer bank</span>{/if}
          </div>
        {:else}
          {#each a.options, i (i)}
            <div class="option">
              <textarea autocomplete="off"
                rows="5"
                aria-label="Option {i + 1}"
                value={optionText(a, i)}
                oninput={(e) => (edits[key(a, i)] = e.currentTarget.value)}
                disabled={busy}
              ></textarea>
              <div class="actions">
                <button type="button" class="secondary" disabled={busy} onclick={() => copy(optionText(a, i))}>
                  Copy option {i + 1}
                </button>
                <button type="button" disabled={busy} onclick={() => approve(a, i)}>Approve option {i + 1}</button>
              </div>
            </div>
          {/each}
          <button type="button" class="link" disabled={busy} onclick={() => remove(a)}>Delete this draft</button>
        {/if}
      </article>
    {/each}
  {/if}
</div>

<style>
  .answers {
    display: grid;
    gap: 0.6rem;
    align-content: start;
    max-height: calc(100vh - 6rem);
    overflow-y: auto;
  }
  .head {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .head h2 {
    margin: 0 auto 0 0;
  }
  h3 {
    margin: 0.25rem 0;
    font-size: 1rem;
  }
  article,
  .suggestions {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
  }
  .suggestions {
    background: var(--accent-soft);
  }
  .banked + .banked {
    border-top: 1px solid var(--border);
    margin-top: 0.5rem;
  }
  .option {
    display: grid;
    gap: 0.3rem;
    margin-bottom: 0.6rem;
  }
  .option textarea {
    width: 100%;
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .badge {
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    background: var(--accent-soft);
    font-size: 0.85em;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--danger);
    text-decoration: underline;
    cursor: pointer;
  }
  .notice {
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
    background: var(--accent-soft);
  }
</style>
