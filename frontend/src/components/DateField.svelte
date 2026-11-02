<script lang="ts">
  /**
   * A year-month date typed as "YYYY-MM" or "YYYY" (A5: stored structured,
   * never split on "-"). The text is kept separately from the value, so a
   * half-typed date isn't reformatted under the cursor.
   */
  import { onDestroy } from 'svelte';

  import type { YearMonth } from '../lib/api';
  import { formatYearMonth, parseYearMonth } from '../lib/dates';
  import { fieldId, useValidity } from '../lib/formValidity';

  let { label, value = $bindable() }: { label: string; value: YearMonth } = $props();

  const id = fieldId();
  const validity = useValidity();
  let text = $state(formatYearMonth(value));
  let invalid = $state(false);

  function onInput(next: string): void {
    text = next;
    const parsed = parseYearMonth(next);
    invalid = parsed === null;
    if (parsed) value = parsed;
    if (invalid) validity?.add(id);
    else validity?.delete(id);
  }

  onDestroy(() => validity?.delete(id));
</script>

<label class="field">
  <span>{label}</span>
  <input
    type="text"
    inputmode="numeric"
    placeholder="YYYY-MM"
    value={text}
    aria-invalid={invalid}
    oninput={(e) => onInput(e.currentTarget.value)}
  />
  {#if invalid}<small class="error">Use YYYY-MM or YYYY</small>{/if}
</label>

<style>
  input {
    width: 8rem;
  }
</style>
