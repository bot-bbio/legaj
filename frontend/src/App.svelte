<script lang="ts">
  import Nav from './components/Nav.svelte';
  import { DEFAULT_SECTION, sectionLabel, type SectionId } from './lib/nav';

  let active = $state<SectionId>(DEFAULT_SECTION);
</script>

<div class="shell">
  <Nav {active} onselect={(id) => (active = id)} />
  <main>
    <h1>{sectionLabel(active)}</h1>
  </main>
</div>

<style>
  .shell {
    display: flex;
    height: 100vh;
  }
  main {
    flex: 1;
    padding: 1.5rem 2rem;
    overflow: auto;
  }
  h1 {
    margin: 0 0 1rem;
    font-size: 1.5rem;
  }
</style>
