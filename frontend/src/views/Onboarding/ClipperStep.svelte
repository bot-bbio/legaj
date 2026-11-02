<script lang="ts">
  let { onfinish, busy }: { onfinish: () => void; busy: boolean } = $props();
</script>

<h2>Clip jobs from your browser</h2>
<p>
  The LeGaJ clipper is a bookmark: on any job posting, click it and LeGaJ saves the job, with its full description, to
  your Pipeline. LeGaJ has to be running for the clipper to work.
</p>
<p>
  To use it, show your browser’s bookmarks bar with <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>B</kbd>, then drag the clipper
  button onto it.
</p>
<p class="muted">
  The clipper button arrives in a later build. Until then, add jobs with <b>Paste job</b> on the Pipeline page. This step
  stays on your setup checklist.
</p>

<div class="actions">
  <button type="button" disabled={busy} onclick={onfinish}>Finish</button>
</div>

<style>
  kbd {
    padding: 0.05rem 0.35rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 4px;
    font-family: inherit;
    font-size: 0.9em;
  }
</style>
