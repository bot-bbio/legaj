<script lang="ts">
  import { api } from '../../lib/api';
  import { errorMessage } from '../../lib/errors';

  let { alreadySaved, onnext }: { alreadySaved: boolean; onnext: () => void } = $props();

  const studioURL = 'https://aistudio.google.com/apikey';

  // The key never stays in component state after submit (R4).
  let keyInput = $state('');
  let busy = $state(false);
  let result = $state('');
  let failure = $state('');
  let saved = $state(false);

  async function submit(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    const key = keyInput;
    keyInput = '';
    busy = true;
    failure = '';
    result = '';
    try {
      const r = await api.saveKeyAndTest(key);
      result = r.message;
      saved = true;
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  async function copyAddress(): Promise<void> {
    try {
      await navigator.clipboard.writeText(studioURL);
    } catch {
      // Clipboard unavailable: the address is shown for manual copying.
    }
  }
</script>

<h2>Connect Gemini</h2>
<p>
  LeGaJ writes cover letters and form answers with Google’s Gemini, using your own free API key. Nobody else pays for
  or sees your usage.
</p>
<ol>
  <li>
    Open <code class="select">{studioURL}</code>
    <button type="button" class="secondary small" onclick={copyAddress}>Copy address</button> in your browser and sign in
    with a Google account.
  </li>
  <li>Click <b>Create API key</b>, then copy the key.</li>
  <li>Paste it below. LeGaJ tests it, then stores it in Windows Credential Manager, never in a file.</li>
</ol>

<aside class="notice">
  <strong>How your data is used.</strong> On Gemini’s <b>free tier</b>, Google may use what LeGaJ sends, including your
  resume text and job descriptions, to improve its products, and <b>human reviewers may read it</b>. On a paid tier,
  Google doesn’t use it that way. LeGaJ sends data only to Gemini, with your key, and keeps everything else on this PC.
</aside>

{#if alreadySaved && !saved}
  <p class="muted">A key is already saved. You can replace it here or go on.</p>
{/if}

<form class="cols" onsubmit={submit}>
  <label class="field grow">
    <span>Gemini API key</span>
    <input type="password" autocomplete="off" spellcheck="false" bind:value={keyInput} disabled={busy} />
  </label>
  <button type="submit" disabled={busy || keyInput.trim() === ''}>{busy ? 'Testing…' : 'Save and test'}</button>
</form>
{#if result}<p class="ok" role="status">{result}</p>{/if}
{#if failure}<p class="error" role="alert">{failure}</p>{/if}

<div class="actions">
  <button type="button" disabled={!(saved || alreadySaved)} onclick={onnext}>Next</button>
  <button type="button" class="secondary" onclick={onnext}>Skip for now</button>
</div>

<style>
  .grow {
    flex: 1;
  }
  .notice {
    margin: 1rem 0;
    padding: 0.75rem 1rem;
    border-left: 4px solid var(--accent);
    background: var(--accent-soft);
    border-radius: 6px;
  }
  .select {
    user-select: all;
  }
  .small {
    padding: 0.15rem 0.5rem;
    font-size: 0.85em;
  }
  .ok {
    color: var(--accent);
  }
</style>
