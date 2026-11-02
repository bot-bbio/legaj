<script lang="ts">
  /**
   * Check what Gemini read from the resume (or start from the saved
   * profile) and say what job you want. The target role is saved exactly
   * as typed (R7, U1).
   */
  import { onMount } from 'svelte';

  import { api, type Profile } from '../../lib/api';
  import { formatYearMonth } from '../../lib/dates';
  import { errorMessage } from '../../lib/errors';
  import { mergeDraft, replacesHistory } from '../../lib/profileMerge';

  let { draft, onnext }: { draft: Profile | null; onnext: () => void } = $props();

  let base = $state<Profile | null>(null);
  let failure = $state('');
  let busy = $state(false);
  let replacing = $state(false);

  // A new resume is merged into what's saved, never pasted over it: the
  // user may have filled the profile by hand before uploading (gate CR1).
  onMount(() => {
    api
      .getProfile()
      .then((saved) => {
        if (draft) {
          const d = $state.snapshot(draft);
          replacing = replacesHistory(saved, d);
          base = mergeDraft(saved, d);
        } else {
          base = saved;
        }
      })
      .catch((err: unknown) => (failure = errorMessage(err)));
  });

  async function save(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    if (!base) return;
    busy = true;
    failure = '';
    try {
      await api.saveProfile($state.snapshot(base));
      onnext();
    } catch (err) {
      failure = errorMessage(err);
    } finally {
      busy = false;
    }
  }

  const span = (s: string, e: string): string => [s, e].filter((x) => x !== '').join(' to ');
</script>

<h2>Check your profile</h2>
{#if !base}
  {#if failure}<p class="error">{failure}</p>{:else}<p class="muted">Loading…</p>{/if}
{:else}
  <p>
    {draft ? 'This is what Gemini read from your resume.' : 'Fill in the basics now; the rest can wait.'} You can edit
    everything later on the Profile page.
  </p>
  {#if replacing}
    <p class="notice" role="note">
      Saving replaces the experience, education or skills on your Profile page with what Gemini read. Your target role,
      links and anything the resume doesn’t mention are kept.
    </p>
  {/if}
  <form onsubmit={save}>
    <div class="cols">
      <label class="field"><span>Full name</span><input type="text" bind:value={base.fullName} /></label>
      <label class="field"><span>Email</span><input type="email" bind:value={base.email} /></label>
      <label class="field"><span>Phone</span><input type="tel" bind:value={base.phone} /></label>
      <label class="field"><span>Location</span><input type="text" bind:value={base.location} /></label>
    </div>
    <label class="field">
      <span>Target role (the job you’re looking for)</span>
      <input type="text" bind:value={base.targetRole} />
    </label>

    {#if base.experiences.length > 0}
      <h3>Experience</h3>
      <ul>
        {#each base.experiences as e (e)}
          <li>
            <b>{e.title}</b>{e.company ? `, ${e.company}` : ''}
            <span class="muted">{span(formatYearMonth(e.start), e.current ? 'now' : formatYearMonth(e.end))}</span>
          </li>
        {/each}
      </ul>
    {/if}
    {#if base.education.length > 0}
      <h3>Education</h3>
      <ul>
        {#each base.education as e (e)}
          <li><b>{e.degree}</b>{e.school ? `, ${e.school}` : ''}</li>
        {/each}
      </ul>
    {/if}
    {#if base.skills.length > 0}
      <h3>Skills</h3>
      <p>{base.skills.flatMap((g) => g.items).join(', ')}</p>
    {/if}

    {#if failure}<p class="error" role="alert">{failure}</p>{/if}
    <div class="actions">
      <button type="submit" disabled={busy}>Save profile</button>
      <button type="button" class="secondary" onclick={onnext}>Skip for now</button>
    </div>
  </form>
{/if}
