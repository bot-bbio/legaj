<script lang="ts">
  import EducationList from '../components/EducationList.svelte';
  import ExperienceList from '../components/ExperienceList.svelte';
  import SkillsEditor from '../components/SkillsEditor.svelte';
  import { api, type Profile } from '../lib/api';
  import { errorMessage } from '../lib/errors';
  import { createValidity } from '../lib/formValidity';
  import { profileStore } from '../lib/stores/profile';

  // The form edits a draft that follows the store until the user changes
  // something (R3). Saving sends the whole profile (R9).
  let draft = $state<Profile | null>(null);
  let dirty = $state(false);
  let saving = $state(false);
  let saved = $state(false);
  let saveError = $state('');
  const invalid = createValidity();

  // Replace the draft only when the stored profile actually changed: a
  // refresh with identical data must not re-mount inputs and steal focus.
  let applied = '';
  $effect(() => {
    const s = $profileStore;
    if (s.status !== 'ready' || dirty) return;
    const json = JSON.stringify(s.data);
    if (json !== applied) {
      applied = json;
      draft = structuredClone(s.data);
    }
  });

  function markDirty(): void {
    dirty = true;
    saved = false;
  }

  async function save(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    if (!draft || invalid.size > 0) return;
    saving = true;
    saveError = '';
    try {
      const stored = await api.saveProfile($state.snapshot(draft));
      applied = JSON.stringify(stored);
      draft = structuredClone(stored);
      dirty = false;
      saved = true;
    } catch (err) {
      saveError = errorMessage(err);
    } finally {
      saving = false;
    }
  }

  function addLink(): void {
    draft?.links.push({ label: '', url: '' });
    markDirty();
  }

  function removeLink(index: number): void {
    draft?.links.splice(index, 1);
    markDirty();
  }
</script>

<h1>Profile</h1>

{#if $profileStore.status === 'loading'}
  <p class="muted">Loading…</p>
{:else if $profileStore.status === 'error'}
  <p class="error">{$profileStore.message}</p>
{:else if draft}
  <form onsubmit={save} oninput={markDirty} onchange={markDirty}>
    <section class="card">
      <h2>About you</h2>
      <div class="cols">
        <label class="field"><span>Full name</span><input type="text" autocomplete="name" bind:value={draft.fullName} /></label>
        <label class="field"><span>Email</span><input type="email" autocomplete="email" bind:value={draft.email} /></label>
        <label class="field"><span>Phone</span><input type="tel" autocomplete="tel" bind:value={draft.phone} /></label>
      </div>
      <div class="cols">
        <label class="field"><span>Location</span><input type="text" bind:value={draft.location} /></label>
        <label class="field"><span>Headline</span><input type="text" bind:value={draft.headline} /></label>
        <label class="field"><span>Target role</span><input type="text" bind:value={draft.targetRole} /></label>
      </div>
      <label class="field"><span>Summary</span><textarea rows="3" bind:value={draft.summary}></textarea></label>
      <h3>Links</h3>
      {#each draft.links as link, i (link)}
        <div class="cols">
          <label class="field"><span>Label</span><input type="text" bind:value={link.label} /></label>
          <label class="field grow"><span>Web address</span><input type="url" bind:value={link.url} /></label>
          <button type="button" class="secondary" onclick={() => removeLink(i)}>Remove link</button>
        </div>
      {/each}
      <p><button type="button" class="secondary" onclick={addLink}>Add link</button></p>
    </section>

    <section class="card">
      <h2>Experience</h2>
      <ExperienceList bind:items={draft.experiences} onchange={markDirty} />
    </section>

    <section class="card">
      <h2>Education</h2>
      <EducationList bind:items={draft.education} onchange={markDirty} />
    </section>

    <section class="card">
      <h2>Skills</h2>
      <SkillsEditor bind:groups={draft.skills} onchange={markDirty} />
    </section>

    <div class="bar">
      <button type="submit" disabled={!dirty || saving || invalid.size > 0}>Save profile</button>
      {#if invalid.size > 0}<span class="error">Fix the highlighted dates first.</span>{/if}
      {#if saved}<span class="muted">Saved.</span>{/if}
      {#if saveError}<span class="error" role="alert">{saveError}</span>{/if}
    </div>
  </form>
{/if}

<style>
  form {
    max-width: 56rem;
  }
  .grow {
    flex: 1;
  }
  .bar {
    position: sticky;
    bottom: 0;
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 0;
    background: var(--bg);
  }
</style>
