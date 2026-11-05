<script lang="ts">
  import Bookmarklet from './Bookmarklet.svelte';

  let { onfinish, busy }: { onfinish: () => void; busy: boolean } = $props();
</script>

<h2>Clip jobs from your browser</h2>
<p>
  The LeGaJ clipper is a bookmark: on any job posting, click it and LeGaJ saves the job, with its full description, to
  your Pipeline.
</p>
<Bookmarklet />
<p class="muted">This step stays on your setup checklist until your first clip.</p>

<div class="actions">
  <button type="button" disabled={busy} onclick={onfinish}>Finish</button>
</div>
