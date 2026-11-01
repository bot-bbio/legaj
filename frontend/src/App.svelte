<script lang="ts">
  import Nav from './components/Nav.svelte';
  import StartupGate from './components/StartupGate.svelte';
  import { navigate, route } from './lib/stores/route';
  import FollowUps from './views/FollowUps.svelte';
  import Interviews from './views/Interviews.svelte';
  import Pipeline from './views/Pipeline.svelte';
  import Profile from './views/Profile.svelte';
  import Settings from './views/Settings.svelte';
</script>

<StartupGate>
  <div class="shell">
    <Nav active={$route} onselect={navigate} />
    <main>
      {#if $route === 'profile'}
        <Profile />
      {:else if $route === 'pipeline'}
        <Pipeline />
      {:else if $route === 'followups'}
        <FollowUps />
      {:else if $route === 'interviews'}
        <Interviews />
      {:else}
        <Settings />
      {/if}
    </main>
  </div>
</StartupGate>

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
  main :global(h1) {
    margin: 0 0 1rem;
    font-size: 1.5rem;
  }
</style>
