<script lang="ts">
  /**
   * The clipper's install page (R18), shown by the setup wizard and by
   * Settings. The bookmarklet has the clip server's port built in, so it's
   * generated here from the current status.
   *
   * The install is recorded when the user drags or copies it, never when the
   * page opens: the reinstall banner clears on that record, and a page that
   * was only looked at leaves the old, broken bookmark in the browser.
   */
  import { bookmarkletURL } from '../../bookmarklet/bookmarklet';
  import { api } from '../../lib/api';
  import { errorMessage } from '../../lib/errors';
  import { clipStore } from '../../lib/stores/clip';

  let copied = $state(false);
  let failure = $state('');

  const href = $derived(
    $clipStore.status === 'ready' && $clipStore.data.running ? bookmarkletURL($clipStore.data.port) : '',
  );

  async function recordInstall(): Promise<void> {
    failure = '';
    try {
      await api.markBookmarkletInstalled();
    } catch (err) {
      failure = errorMessage(err);
    }
  }

  async function copy(): Promise<void> {
    failure = '';
    copied = false;
    try {
      await api.copyText(href);
    } catch (err) {
      failure = errorMessage(err);
      return;
    }
    copied = true;
    await recordInstall();
  }
</script>

{#if $clipStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $clipStore.status === 'error'}
  <p class="error">{$clipStore.message}</p>
{:else if !href}
  <p class="error">{$clipStore.data.message || 'LeGaJ’s clipper isn’t running. Restart LeGaJ and try again.'}</p>
{:else}
  <ol>
    <li>Show your browser’s bookmarks bar with <kbd>Ctrl+Shift+B</kbd>.</li>
    <li>
      Drag this button onto the bar:
      <!-- Clicked here it would run on LeGaJ's own window, not on a job page. -->
      <a class="clipper" {href} ondragstart={recordInstall} onclick={(e) => e.preventDefault()}>Clip to LeGaJ</a>
    </li>
    <li>On a job posting, click the new bookmark, then click <b>Save</b> in the window that opens.</li>
  </ol>
  <p class="muted">
    Can’t drag it? Copy it, add a new bookmark in your browser, and paste this as the bookmark’s address. If you already
    have an older clipper bookmark, delete it.
  </p>
  <p class="row">
    <button type="button" class="secondary" onclick={copy}>Copy the clipper</button>
    {#if copied}<span class="muted" role="status">Copied.</span>{/if}
  </p>
  <p>
    LeGaJ has to be running for the clipper to work. To have it ready whenever you sign in, turn on
    <b>Start LeGaJ with Windows</b> in Settings.
  </p>
  <p class="muted">On a site LeGaJ doesn’t know, select the job description first: it helps LeGaJ find the details.</p>
{/if}
{#if failure}<p class="error" role="alert">{failure}</p>{/if}

<style>
  ol {
    margin: 0 0 0.75rem;
    padding-left: 1.25rem;
  }
  li {
    margin-bottom: 0.5rem;
  }
  kbd {
    padding: 0.05rem 0.35rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 4px;
    font-family: inherit;
    font-size: 0.9em;
  }
  .clipper {
    display: inline-block;
    margin-left: 0.35rem;
    padding: 0.3rem 0.8rem;
    border-radius: 6px;
    background: var(--accent);
    color: #fff;
    font-weight: 600;
    text-decoration: none;
    cursor: grab;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  button {
    font: inherit;
    padding: 0.45rem 0.9rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }
  .muted {
    color: var(--muted);
  }
  .error {
    color: var(--danger);
  }
</style>
