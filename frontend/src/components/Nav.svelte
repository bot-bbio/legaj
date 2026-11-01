<script lang="ts">
  import { NAV_SECTIONS, type SectionId } from '../lib/nav';

  interface Props {
    active: SectionId;
    onselect: (id: SectionId) => void;
  }

  let { active, onselect }: Props = $props();
</script>

<nav aria-label="Main">
  <ul>
    {#each NAV_SECTIONS as section (section.id)}
      <li>
        <button
          type="button"
          aria-current={section.id === active ? 'page' : undefined}
          onclick={() => onselect(section.id)}
        >
          {section.label}
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
</style>
