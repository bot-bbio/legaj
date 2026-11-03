<script lang="ts">
  import { onMount } from 'svelte';

  import { NAV_SECTIONS, type SectionId } from '../lib/nav';
  import { DUE_REFRESH_MS, dueStore } from '../lib/stores/due';

  interface Props {
    active: SectionId;
    onselect: (id: SectionId) => void;
  }

  let { active, onselect }: Props = $props();

  // The Follow-ups badge counts what's due now (R28).
  const dueCount = $derived($dueStore.status === 'ready' ? $dueStore.data.length : 0);

  onMount(() => {
    const timer = setInterval(() => void dueStore.refresh(), DUE_REFRESH_MS);
    return () => clearInterval(timer);
  });

  const label = (id: SectionId, text: string): string =>
    id === 'followups' && dueCount > 0 ? `${text}, ${dueCount} due` : text;
</script>

<nav aria-label="Main">
  <ul>
    {#each NAV_SECTIONS as section (section.id)}
      <li>
        <button
          type="button"
          aria-current={section.id === active ? 'page' : undefined}
          aria-label={label(section.id, section.label)}
          onclick={() => onselect(section.id)}
        >
          {section.label}
          {#if section.id === 'followups' && dueCount > 0}
            <span class="badge" aria-hidden="true">{dueCount}</span>
          {/if}
        </button>
      </li>
    {/each}
  </ul>
</nav>

<style>
  nav {
    width: 13rem;
    padding: 1rem 0.75rem;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  button {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    text-align: left;
    padding: 0.55rem 0.75rem;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }
  button:hover {
    background: var(--hover);
  }
  button[aria-current='page'] {
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }
  .badge {
    min-width: 1.4rem;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--accent);
    color: var(--surface);
    font-size: 0.8em;
    font-weight: 600;
    text-align: center;
  }
</style>
