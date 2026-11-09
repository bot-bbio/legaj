/**
 * Pipeline layout in a real browser (R46, R50, R70).
 *
 * Item 5 proves the harness on the V2 bugs the user hit: they're asserted as
 * expected failures (`test.fail`), so this run passes only while the checks
 * still see them. The checks' own correctness is shown in
 * layout-checks.e2e.ts, and a screen V2 gets right is the control.
 * Item 25 rebuilds the table, removes the `test.fail` marks and widens the
 * matrix to every viewport and scheme.
 */
import { expect, test, type Page } from '@playwright/test';

import { expectAllHandled, installFakeBackend } from './fake-backend';
import { baseData } from './fixtures';
import { SCHEMES, VIEWPORTS, headerMisalignment, horizontalOverflow, sideColumns, snapshot } from './layout';

async function openSection(page: Page, section: 'Pipeline' | 'Profile'): Promise<void> {
  await installFakeBackend(page, baseData(60));
  await page.goto('/');
  await page.getByRole('navigation', { name: 'Main' }).getByRole('button', { name: section }).click();
  await expect(page.getByRole('heading', { level: 1, name: section })).toBeVisible();
}

async function openPipeline(page: Page): Promise<void> {
  await openSection(page, 'Pipeline');
  await expect(page.getByRole('table', { name: 'Applications' })).toBeVisible();
}

for (const viewport of [VIEWPORTS[0], VIEWPORTS[2]]) {
  test.describe(`V2 regressions at ${viewport.width}×${viewport.height} (item 25 fixes them)`, () => {
    test.use({ viewport });

    test('R46: nothing scrolls or clips sideways', async ({ page }) => {
      test.fail(true, "V2: the row links don't fit their 18rem column, and below ~1,330px the columns don't fit at all");
      await openPipeline(page);
      expect(await horizontalOverflow(page)).toEqual([]);
    });

    test('R46: headers stay over their columns after scrolling right', async ({ page }) => {
      test.fail(true, "V2: the body scrolls sideways under a header row that doesn't (C52)");
      await openPipeline(page);
      expect(await headerMisalignment(page, 'Applications')).toEqual([]);
    });
  });
}

test.describe('V2 Review side panel (item 30 removes it)', () => {
  test.use({ viewport: VIEWPORTS[1] });

  test('R50: no side column', async ({ page }) => {
    test.fail(true, 'V2: Review opens as a 20–26rem column beside the table');
    await openPipeline(page);
    await page.getByRole('button', { name: 'Review', exact: true }).click();
    await expect(page.getByRole('complementary', { name: 'Review' })).toBeVisible();
    expect(await sideColumns(page)).toEqual([]);
  });
});

test.describe('control: a screen V2 gets right', () => {
  test.use({ viewport: VIEWPORTS[0] });

  test('Profile at 1280×720', async ({ page }) => {
    await openSection(page, 'Profile');
    expect(await horizontalOverflow(page)).toEqual([]);
    expect(await sideColumns(page)).toEqual([]);
    await expectAllHandled(page);
  });
});

for (const viewport of VIEWPORTS) {
  for (const scheme of SCHEMES) {
    test.describe(`screenshot ${viewport.width}×${viewport.height} ${scheme}`, () => {
      test.use({ viewport, colorScheme: scheme });
      test('Pipeline', async ({ page }) => {
        await openPipeline(page);
        await snapshot(page, 'pipeline', scheme);
        await expectAllHandled(page);
      });
    });
  }
}
