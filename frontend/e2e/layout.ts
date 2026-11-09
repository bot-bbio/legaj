/**
 * Layout checks for the real-browser suite (R70, C56). Each returns a list of
 * human-readable problems, empty when the layout is right, so a failure says
 * what is wrong and where. jsdom has no layout engine; these run in Edge,
 * the engine WebView2 uses.
 */
import type { Page } from '@playwright/test';

export const VIEWPORTS = [
  { width: 1280, height: 720 },
  { width: 1366, height: 768 },
  { width: 1920, height: 1080 },
] as const;

export const SCHEMES = ['light', 'dark'] as const;

/**
 * Elements whose content is wider than their box, so they scroll or clip
 * sideways (G7). Text cut on purpose with an ellipsis, and form fields
 * holding long values, are not overflow.
 */
export async function horizontalOverflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const describe = (el: Element): string => {
      const role = el.getAttribute('role');
      const label = el.getAttribute('aria-label');
      const cls = typeof el.className === 'string' && el.className.trim() ? '.' + el.className.trim().split(/\s+/).join('.') : '';
      return el.tagName.toLowerCase() + cls + (role ? `[role=${role}]` : '') + (label ? `[aria-label="${label}"]` : '');
    };
    const out: string[] = [];
    const doc = document.scrollingElement;
    if (doc && doc.scrollWidth > doc.clientWidth + 1) out.push(`the page (${doc.scrollWidth}px > ${doc.clientWidth}px)`);
    for (const el of Array.from(document.querySelectorAll('body *'))) {
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) continue;
      const s = getComputedStyle(el);
      if (s.display === 'none' || s.overflowX === 'visible' || s.textOverflow === 'ellipsis') continue;
      if (el.scrollWidth > el.clientWidth + 1) out.push(`${describe(el)} (${el.scrollWidth}px > ${el.clientWidth}px)`);
    }
    return out;
  });
}

/**
 * Columns whose header doesn't sit over its cells. Every part of the table a
 * user can scroll sideways is first scrolled fully right, as a user would;
 * V2's body scrolled while its header row stayed put (C52).
 */
export async function headerMisalignment(page: Page, tableLabel: string): Promise<string[]> {
  return page.evaluate((label) => {
    const table = document.querySelector(`[role=table][aria-label="${label}"]`);
    if (!table) return [`no table labelled "${label}"`];
    for (const el of [table, ...Array.from(table.querySelectorAll('*'))]) {
      const x = getComputedStyle(el).overflowX;
      if ((x === 'auto' || x === 'scroll') && el.scrollWidth > el.clientWidth) el.scrollLeft = el.scrollWidth;
    }
    const headers = Array.from(table.querySelectorAll('[role=columnheader]'));
    const row = Array.from(table.querySelectorAll('[role=row]')).find((r) => r.querySelector('[role=cell]'));
    const cells = row ? Array.from(row.querySelectorAll(':scope > [role=cell]')) : [];
    if (headers.length === 0 || headers.length !== cells.length) {
      return [`${headers.length} header cells but ${cells.length} cells in the first row`];
    }
    const out: string[] = [];
    headers.forEach((h, i) => {
      const a = h.getBoundingClientRect();
      const b = cells[i]?.getBoundingClientRect();
      if (!b) return;
      if (Math.abs(a.left - b.left) > 1 || Math.abs(a.right - b.right) > 1) {
        const name = h.textContent?.trim() || `column ${i + 1}`;
        out.push(`"${name}": header ${Math.round(a.left)}–${Math.round(a.right)}px, cells ${Math.round(b.left)}–${Math.round(b.right)}px`);
      }
    });
    return out;
  }, tableLabel);
}

/**
 * Side panels: complementary regions laid out as a column beside other
 * content instead of across the page (R50). The Files tab's details pane is
 * the one allowed split and is marked `data-allowed-split`.
 */
export async function sideColumns(page: Page): Promise<string[]> {
  return page.evaluate(() =>
    Array.from(document.querySelectorAll('[role=complementary], aside')).flatMap((el) => {
      if (el.closest('[data-allowed-split]')) return [];
      const r = el.getBoundingClientRect();
      const parent = el.parentElement?.getBoundingClientRect();
      if (!parent || r.width === 0) return [];
      const name = el.getAttribute('aria-label') ?? el.tagName.toLowerCase();
      return r.width < parent.width * 0.9 ? [`"${name}" is a ${Math.round(r.width)}px column in a ${Math.round(parent.width)}px area`] : [];
    }),
  );
}

/** Saves a screenshot for the gate's visual review (R70). */
export async function snapshot(page: Page, name: string, scheme: string): Promise<void> {
  const { width, height } = page.viewportSize() ?? { width: 0, height: 0 };
  await page.screenshot({ path: `e2e/out/screens/${name}-${width}x${height}-${scheme}.png`, fullPage: false });
}
