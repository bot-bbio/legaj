<script lang="ts">
  import About from '../components/About.svelte';
  import { api, type Settings } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { settingsStore } from '../lib/stores/settings';

  // The form edits a draft. Until the user changes something it follows the
  // store, so a change made elsewhere shows up here at once (R3).
  let draft = $state<Settings | null>(null);
  let dirty = $state(false);
  let saving = $state(false);
  let saveError = $state('');
  let saved = $state(false);

  $effect(() => {
    const s = $settingsStore;
    if (s.status === 'ready' && !dirty) draft = { ...s.data.settings };
  });

  async function saveSettings(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    if (!draft) return;
    saving = true;
    saveError = '';
    saved = false;
    try {
      const view = await api.saveSettings({ ...draft });
      draft = { ...view.settings };
      dirty = false;
      saved = true;
    } catch (err) {
      saveError = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  // The API key never lives in component state longer than one submit (R4).
  let keyInput = $state('');
  let keyBusy = $state(false);
  let keyError = $state('');
  let confirmRemove = $state(false);

  async function saveKey(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    const key = keyInput;
    keyInput = '';
    keyBusy = true;
    keyError = '';
    try {
      await api.saveApiKey(key);
    } catch (err) {
      keyError = errorMessage(err);
    } finally {
      keyBusy = false;
    }
  }

  async function removeKey(): Promise<void> {
    confirmRemove = false;
    keyBusy = true;
    keyError = '';
    try {
      await api.removeApiKey();
    } catch (err) {
      keyError = errorMessage(err);
    } finally {
      keyBusy = false;
    }
  }
</script>

<h1>Settings</h1>

{#if $settingsStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $settingsStore.status === 'error'}
  <p class="error">{$settingsStore.message}</p>
{:else}
  {@const key = $settingsStore.data.key}
  <section>
    <h2>Gemini API key</h2>
    <p>
      {#if key.saved}
        Saved in Windows Credential Manager: <span class="mask">{key.masked}</span>
      {:else}
        No key saved. LeGaJ needs one to write letters and answers.
      {/if}
    </p>
    <form class="row" onsubmit={saveKey}>
      <label class="grow">
        <span>New API key</span>
        <input type="password" autocomplete="off" spellcheck="false" bind:value={keyInput} disabled={keyBusy} />
      </label>
      <button type="submit" disabled={keyBusy || keyInput.trim() === ''}>{key.saved ? 'Replace key' : 'Save key'}</button>
    </form>
    {#if key.saved}
      {#if confirmRemove}
        <p class="row">
          Remove the key? LeGaJ can’t generate anything until you add one again.
          <button type="button" class="danger" onclick={removeKey}>Yes, remove</button>
          <button type="button" class="secondary" onclick={() => (confirmRemove = false)}>Cancel</button>
        </p>
      {:else}
        <p><button type="button" class="secondary" disabled={keyBusy} onclick={() => (confirmRemove = true)}>Remove key</button></p>
      {/if}
    {/if}
    {#if keyError}<p class="error" role="alert">{keyError}</p>{/if}
  </section>

  {#if draft}
    <section>
      <h2>Preferences</h2>
      <form class="grid" onsubmit={saveSettings} oninput={() => ((dirty = true), (saved = false))}>
        <label>
          <span>Gemini model</span>
          <input type="text" spellcheck="false" bind:value={draft.model} />
        </label>
        <label>
          <span>Default materials for new applications</span>
          <select bind:value={draft.defaultMaterials} onchange={() => (dirty = true)}>
            <option value="letter">Cover letter</option>
            <option value="answers">Form answers</option>
            <option value="both">Both</option>
          </select>
        </label>
        <label>
          <span>Follow up after (days)</span>
          <input type="number" min="1" max="60" bind:value={draft.followUpDays} />
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={draft.skipReview} onchange={() => (dirty = true)} />
          <span>Skip review: make PDFs straight away when generating in bulk</span>
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={draft.updateCheck} onchange={() => (dirty = true)} />
          <span>Check GitHub for new versions</span>
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={draft.startWithWindows} onchange={() => (dirty = true)} />
          <span>Start LeGaJ with Windows (so the clipper works right after you sign in)</span>
        </label>
        <p class="row">
          <button type="submit" disabled={!dirty || saving}>Save settings</button>
          {#if saved}<span class="muted">Saved.</span>{/if}
        </p>
        {#if saveError}<p class="error" role="alert">{saveError}</p>{/if}
      </form>
    </section>
  {/if}
{/if}

<About />

<style>
  section {
    max-width: 44rem;
    margin-bottom: 2rem;
    padding: 1.25rem 1.5rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
  }
  h2 {
    margin: 0 0 0.75rem;
    font-size: 1.1rem;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  label.check {
    flex-direction: row;
    align-items: center;
    gap: 0.5rem;
  }
  .grid {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
  }
  .row {
    display: flex;
    align-items: flex-end;
    gap: 0.75rem;
    flex-wrap: wrap;
  }
  .grow {
    flex: 1;
  }
  input[type='text'],
  input[type='password'],
  input[type='number'],
  select {
    font: inherit;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
  }
  input[type='number'] {
    width: 6rem;
  }
  button {
    font: inherit;
    padding: 0.45rem 0.9rem;
    border-radius: 6px;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: #fff;
    cursor: pointer;
  }
  button.secondary {
    background: transparent;
    color: var(--text);
    border-color: var(--border);
  }
  button.danger {
    background: var(--danger);
    border-color: var(--danger);
  }
  button:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .mask {
    font-family: Consolas, monospace;
  }
  .muted {
    color: var(--muted);
  }
  .error {
    color: var(--danger);
  }
</style>
