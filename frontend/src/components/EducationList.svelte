<script lang="ts">
  import type { Education } from '../lib/api';
  import DateField from './DateField.svelte';

  let { items = $bindable(), onchange }: { items: Education[]; onchange: () => void } = $props();

  const blank = (): Education => ({
    id: '',
    school: '',
    degree: '',
    field: '',
    location: '',
    start: { year: 0, month: 0 },
    end: { year: 0, month: 0 },
    details: '',
  });

  function add(): void {
    items.push(blank());
    onchange();
  }

  function remove(index: number): void {
    items.splice(index, 1);
    onchange();
  }
</script>

{#each items as edu, i (edu)}
  <fieldset class="item">
    <legend>Education {i + 1}</legend>
    <div class="cols">
      <label class="field"><span>School</span><input type="text" bind:value={edu.school} /></label>
      <label class="field"><span>Degree</span><input type="text" bind:value={edu.degree} /></label>
      <label class="field"><span>Field of study</span><input type="text" bind:value={edu.field} /></label>
    </div>
    <div class="cols">
      <label class="field"><span>Location</span><input type="text" bind:value={edu.location} /></label>
      <DateField label="Start" bind:value={edu.start} />
      <DateField label="End" bind:value={edu.end} />
    </div>
    <label class="field">
      <span>Details (honours, thesis, relevant courses)</span>
      <textarea rows="2" bind:value={edu.details}></textarea>
    </label>
    <p><button type="button" class="secondary" onclick={() => remove(i)}>Remove</button></p>
  </fieldset>
{/each}
<p><button type="button" class="secondary" onclick={add}>Add education</button></p>
