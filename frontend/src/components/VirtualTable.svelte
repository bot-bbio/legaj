<script lang="ts" generics="T extends { id: number }">
  /**
   * A table that renders only the rows in view, plus a small overscan, inside
   * a full-height spacer (R17, C12). 1,000 rows keep about 40 row nodes in
   * the DOM.
   */
  import { untrack, type Snippet } from 'svelte';

  interface Props {
    rows: readonly T[];
    label: string;
    header: Snippet;
    row: Snippet<[T]>;
    rowHeight?: number;
    height?: number;
    overscan?: number;
    /** When set, the table scrolls once so this row is in view, then calls onscrolled. */
    scrollToId?: number | null;
    onscrolled?: () => void;
  }

  let {
    rows,
    label,
    header,
    row,
    rowHeight = 36,
    height = 560,
    overscan = 8,
    scrollToId = null,
    onscrolled,
  }: Props = $props();

  let scrollTop = $state(0);
  let viewport = $state<HTMLDivElement | null>(null);

  // Runs when scrollToId changes, never on later row changes: a refresh
  // must not yank the user back to a row they already moved away from.
  $effect(() => {
    const id = scrollToId;
    const el = viewport;
    if (id === null || !el) return;
    untrack(() => {
      const index = rows.findIndex((r) => r.id === id);
      if (index < 0) return;
      // Put the row a couple of lines below the top edge.
      const top = Math.max(0, (index - 2) * rowHeight);
      el.scrollTop = top;
      scrollTop = top;
      onscrolled?.();
    });
  });
  const start = $derived(Math.max(0, Math.floor(scrollTop / rowHeight) - overscan));
  const end = $derived(Math.min(rows.length, Math.ceil((scrollTop + height) / rowHeight) + overscan));
  const visible = $derived(rows.slice(start, end));
</script>

<div role="table" aria-label={label} aria-rowcount={rows.length + 1} class="table">
  <div role="rowgroup" class="head">
    <div role="row" aria-rowindex={1} class="grid-row">
      {@render header()}
    </div>
  </div>
  <div
    bind:this={viewport}
    class="viewport"
    data-testid="virtual-viewport"
    style:height="{height}px"
    onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
  >
    <div role="rowgroup" class="spacer" style:height="{rows.length * rowHeight}px">
      {#each visible as item, i (item.id)}
        <div
          role="row"
          aria-rowindex={start + i + 2}
          class="grid-row body-row"
          style:top="{(start + i) * rowHeight}px"
          style:height="{rowHeight}px"
        >
          {@render row(item)}
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .table {
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    overflow: hidden;
  }
  .head {
    border-bottom: 1px solid var(--border);
    font-weight: 600;
    color: var(--muted);
  }
  .viewport {
    overflow-y: auto;
    position: relative;
  }
  .spacer {
    position: relative;
  }
  .body-row {
    position: absolute;
    left: 0;
    right: 0;
    border-bottom: 1px solid var(--border);
  }
  .body-row:hover {
    background: var(--hover);
  }
</style>
