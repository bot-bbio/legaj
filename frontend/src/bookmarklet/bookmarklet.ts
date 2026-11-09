import script from 'virtual:legaj-bookmarklet';

/**
 * The bookmark itself: a javascript: URL with the clip server's port built
 * in (R18). The port is the only thing in it that isn't public, and it isn't
 * a secret: the server's checks don't rely on it being unknown.
 */
export function bookmarkletURL(port: number): string {
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`LeGaJ can't build a bookmarklet for port ${port}.`);
  }
  // Percent-encoded so that '#', '%' and spaces survive as a URL.
  return 'javascript:' + encodeURIComponent(script.replaceAll('__LEGAJ_ORIGIN__', `http://127.0.0.1:${port}`));
}
