<script lang="ts">
  /**
   * One application's materials choice (R26): what must be approved before
   * a Lead becomes Ready. The value shown is always the stored one; a failed
   * save puts the select back.
   */
  import { api, type ApplicationRow, type Materials } from '../lib/api';
  import { errorMessage } from '../lib/errors';

  let { row, onerror }: { row: ApplicationRow; onerror: (message: string) => void } = $props();

  const CHOICES: readonly { id: Materials; label: string }[] = [
    { id: 'letter', label: 'Letter' },
    { id: 'answers', label: 'Answers' },
    { id: 'both', label: 'Both' },
  ];

  const isMaterials = (v: string): v is Materials => CHOICES.some((c) => c.id === v);

  async function change(e: Event & { currentTarget: HTMLSelectElement }): Promise<void> {
    const select = e.currentTarget;
    const value = select.value;
    if (!isMaterials(value)) return;
    try {
      await api.setMaterials(row.id, value);
    } catch (err) {
      select.value = row.materials;
      onerror(errorMessage(err));
    }
  }
</script>

<select class="materials" aria-label="Materials for {row.company || 'row'}" value={row.materials} onchange={change}>
  {#if !isMaterials(row.materials)}
    <option value="" disabled>Choose…</option>
  {/if}
  {#each CHOICES as c (c.id)}
    <option value={c.id}>{c.label}</option>
  {/each}
</select>

<style>
  .materials {
    padding: 0.15rem 0.3rem;
    font-size: 0.9em;
  }
</style>
