/**
 * The layout checks themselves, on tiny pages with a known answer, so a pass
 * elsewhere means "the layout is right", not "the check is blind" (R70).
 */
import { expect, test } from '@playwright/test';

import { headerMisalignment, horizontalOverflow, sideColumns } from './layout';

const grid = 'display:grid;grid-template-columns:200px 300px 200px';

test('a table that fits: no overflow, headers aligned', async ({ page }) => {
  await page.setContent(`
    <div role="table" aria-label="T" style="width:800px">
      <div role="row" style="${grid}"><span role="columnheader">A</span><span role="columnheader">B</span><span role="columnheader">C</span></div>
      <div role="row" style="${grid}"><span role="cell">1</span><span role="cell">2</span><span role="cell">3</span></div>
    </div>`);
  expect(await horizontalOverflow(page)).toEqual([]);
  expect(await headerMisalignment(page, 'T')).toEqual([]);
});

test("V2's shape: a body that scrolls sideways under a fixed header row", async ({ page }) => {
  await page.setContent(`
    <div role="table" aria-label="T" style="width:400px;overflow:hidden">
      <div role="row" style="${grid}"><span role="columnheader">A</span><span role="columnheader">B</span><span role="columnheader">C</span></div>
      <div style="overflow-y:auto;height:100px">
        <div role="row" style="${grid}"><span role="cell">1</span><span role="cell">2</span><span role="cell">3</span></div>
      </div>
    </div>`);
  expect(await horizontalOverflow(page)).not.toEqual([]);
  const problems = await headerMisalignment(page, 'T');
  expect(problems.length).toBe(3);
  expect(problems[0]).toContain('"A"');
});

test('text cut on purpose with an ellipsis is not overflow', async ({ page }) => {
  await page.setContent(`
    <div style="width:100px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">a very long company name that does not fit</div>
    <input style="width:60px" value="a very long value that does not fit either">`);
  expect(await horizontalOverflow(page)).toEqual([]);
});

test('a complementary region beside content is a side column; one across the page is not', async ({ page }) => {
  await page.setContent(`
    <div style="display:grid;grid-template-columns:1fr 300px;width:1000px">
      <div>table</div><div role="complementary" aria-label="Review">panel</div>
    </div>
    <div style="width:1000px"><aside aria-label="Checklist">across the page</aside></div>
    <div data-allowed-split style="display:grid;grid-template-columns:1fr 1fr;width:1000px">
      <div>files</div><div role="complementary" aria-label="Details">pane</div>
    </div>`);
  expect(await sideColumns(page)).toEqual(['"Review" is a 300px column in a 1000px area']);
});
