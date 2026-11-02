<script lang="ts">
  import Nav from './components/Nav.svelte';
  import SetupChecklist from './components/SetupChecklist.svelte';
  import StartupGate from './components/StartupGate.svelte';
  import { onboardingStore, openWizard, wizard } from './lib/stores/onboarding';
  import { navigate, route } from './lib/stores/route';
  import FollowUps from './views/FollowUps.svelte';
  import Interviews from './views/Interviews.svelte';
  import Wizard from './views/Onboarding/Wizard.svelte';
  import Pipeline from './views/Pipeline.svelte';
  import Profile from './views/Profile.svelte';
  import Settings from './views/Settings.svelte';

  // Open the wizard once, on first run (R7). After that it only opens from
  // the setup checklist.
  let autoOpened = false;
  $effect(() => {
    const s = $onboardingStore;
    if (s.status === 'ready' && !s.data.finished && !autoOpened) {
      autoOpened = true;
      openWizard('key');
    }
  });
</script>

<StartupGate>
  {#if $onboardingStore.status === 'loading'}
    <p class="muted center">Loading…</p>
  {:else if $wizard.open}
    {#key $wizard.step}
      <Wizard start={$wizard.step} />
    {/key}
  {:else}
    <div class="shell">
      <Nav active={$route} onselect={navigate} />
      <main>
        <SetupChecklist />
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
  {/if}
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
  .center {
    text-align: center;
    margin-top: 30vh;
  }
</style>
