/**
 * Lets a form know whether any of its fields currently holds text it can't
 * accept (e.g. a half-typed date). The form creates the set; fields add
 * their ID while invalid and remove it when fixed or destroyed.
 */
import { getContext, setContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';

const KEY = Symbol('form-validity');

export function createValidity(): SvelteSet<string> {
  const invalid = new SvelteSet<string>();
  setContext(KEY, invalid);
  return invalid;
}

export function useValidity(): SvelteSet<string> | undefined {
  return getContext<SvelteSet<string> | undefined>(KEY);
}

let counter = 0;

export function fieldId(): string {
  counter += 1;
  return `field-${counter}`;
}
