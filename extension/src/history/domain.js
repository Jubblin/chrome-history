// Lower-cased host without port (and without the brackets around an IPv6
// literal, like Go's url.Hostname), or '' when the URL cannot be parsed.
export function domainOf(rawUrl) {
  try {
    return new URL(rawUrl).hostname.toLowerCase().replace(/^\[(.*)\]$/, '$1');
  } catch {
    return '';
  }
}
