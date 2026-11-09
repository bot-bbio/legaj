import { build, type Plugin } from 'vite';

const VIRTUAL = 'virtual:legaj-bookmarklet';
const RESOLVED = '\0' + VIRTUAL;

/**
 * Compiles src/bookmarklet/entry.ts into one minified script and exposes it
 * as the default export of `virtual:legaj-bookmarklet` (R18).
 *
 * It's a plugin rather than a build step so that the app and vitest get the
 * same string: the tests run exactly what a user's bookmark will hold, not
 * a transpiled copy of the source.
 */
export function bookmarklet(): Plugin {
  let root = '';
  return {
    name: 'legaj-bookmarklet',
    configResolved(config) {
      root = config.root;
    },
    resolveId: (id) => (id === VIRTUAL ? RESOLVED : undefined),
    async load(id) {
      if (id !== RESOLVED) return undefined;
      const entry = `${root}/src/bookmarklet/entry.ts`;
      this.addWatchFile(entry);
      this.addWatchFile(`${root}/src/bookmarklet/collector.ts`);
      const result = await build({
        root,
        configFile: false,
        envFile: false,
        logLevel: 'silent',
        build: {
          write: false,
          minify: true,
          lib: { entry, formats: ['iife'], name: 'legajClip' },
        },
      });
      const [bundle] = Array.isArray(result) ? result : [result];
      const code = bundle && 'output' in bundle ? bundle.output[0].code.trim() : '';
      if (code === '') throw new Error('the bookmarklet build produced no script');
      return `export default ${JSON.stringify(code)};`;
    },
  };
}
