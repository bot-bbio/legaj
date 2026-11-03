<script lang="ts">
  import Nav from './components/Nav.svelte';
  import SetupChecklist from './components/SetupChecklist.svelte';
  import StartupGate from './components/StartupGate.svelte';
  import UpdateBanner from './components/UpdateBanner.svelte';
  import { onboardingStore, openWizard, wizard } from './lib/stores/onboarding';
  import { navigate, route } from './lib/stores/route';
  import FollowUps from './views/FollowUps.svelte';
  import Interviews from './views/Interviews.svelte';
  import Wizard from './views/Onboarding/Wizard.svelte';
  import Pipeline from './views/Pipeline.svelte';
  import Profile from './views/Profile.svelte';
  import Settings from './views/Settings.svelte';

  // Open the wizard once, on first run (R7). After that it only opens from
  // the setup checklist. firstRun shows it from the very first frame, so the
  // shell never flashes (and never loads its views) before the wizard.
  let autoOpened = $state(false);
  const firstRun = $derived($onboardingStore.status === 'ready' && !$onboardingStore.data.finished && !autoOpened);
  $effect(() => {
    if (firstRun) {
      autoOpened = true;
      openWizard('key');
    }
  });
</script>

<StartupGate>
  {#if $onboardingStore.status === 'loading'}
    <p class="muted center">Loading…</p>
  {:else if $wizard.open || firstRun}
    {#key $wizard.step}
      <Wizard start={$wizard.open ? $wizard.step : 'key'} />
    {/key}
  {:else}
    <div class="shell">
      <Nav active={$route} onselect={navigate} />
      <main>
        <UpdateBanner />
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
