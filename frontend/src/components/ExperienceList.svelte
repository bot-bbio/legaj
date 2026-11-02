<script lang="ts">
  import type { Experience } from '../lib/api';
  import DateField from './DateField.svelte';

  let { items = $bindable(), onchange }: { items: Experience[]; onchange: () => void } = $props();

  const blank = (): Experience => ({
    id: '',
    company: '',
    title: '',
    location: '',
    start: { year: 0, month: 0 },
    end: { year: 0, month: 0 },
    current: false,
    bullets: [],
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

{#each items as exp, i (exp)}
  <fieldset class="item">
    <legend>Experience {i + 1}</legend>
    <div class="cols">
      <label class="field"><span>Company</span><input type="text" bind:value={exp.company} /></label>
      <label class="field"><span>Title</span><input type="text" bind:value={exp.title} /></label>
      <label class="field"><span>Location</span><input type="text" bind:value={exp.location} /></label>
    </div>
    <div class="cols">
      <DateField label="Start" bind:value={exp.start} />
      {#if !exp.current}
        <DateField label="End" bind:value={exp.end} />
      {/if}
      <label class="check">
        <input
          type="checkbox"
          bind:checked={exp.current}
          onchange={() => {
            if (exp.current) exp.end = { year: 0, month: 0 };
          }}
        />
        <span>I work here now</span>
      </label>
    </div>
    <label class="field">
      <span>What you did (one point per line)</span>
      <textarea rows="4" value={exp.bullets.join('\n')} oninput={(e) => (exp.bullets = e.currentTarget.value.split('\n'))}
      ></textarea>
    </label>
    <p><button type="button" class="secondary" onclick={() => remove(i)}>Remove</button></p>
  </fieldset>
{/each}
<p><button type="button" class="secondary" onclick={add}>Add experience</button></p>
