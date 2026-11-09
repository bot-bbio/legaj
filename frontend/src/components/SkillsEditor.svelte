<script lang="ts">
  /**
   * Skill groups. Items are typed as comma-separated text. The text is kept
   * as typed (per group) and the parsed list is written to the group, so a
   * comma is never undone by re-rendering.
   */
  import type { SkillGroup } from '../lib/api';

  let { groups = $bindable(), onchange }: { groups: SkillGroup[]; onchange: () => void } = $props();

  const typed = new WeakMap<SkillGroup, string>();

  function itemsText(group: SkillGroup): string {
    return typed.get(group) ?? group.items.join(', ');
  }

  function onItems(group: SkillGroup, next: string): void {
    typed.set(group, next);
    group.items = next
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s !== '');
  }

  function add(): void {
    groups.push({ category: '', items: [] });
    onchange();
  }

  function remove(index: number): void {
    groups.splice(index, 1);
    onchange();
  }
</script>

{#each groups as group, i (group)}
  <fieldset class="item">
    <legend>Skill group {i + 1}</legend>
    <div class="cols">
      <label class="field"><span>Category</span><input autocomplete="off" type="text" bind:value={group.category} /></label>
      <label class="field grow">
        <span>Skills (comma-separated)</span>
        <input autocomplete="off" type="text" value={itemsText(group)} oninput={(e) => onItems(group, e.currentTarget.value)} />
      </label>
    </div>
    <p><button type="button" class="secondary" onclick={() => remove(i)}>Remove</button></p>
  </fieldset>
{/each}
<p><button type="button" class="secondary" onclick={add}>Add skill group</button></p>

<style>
  .grow {
    flex: 1;
  }
</style>
