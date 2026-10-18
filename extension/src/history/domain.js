// Lower-cased host without port, or '' when the URL cannot be parsed.
export function domainOf(rawUrl) {
  try {
    return new URL(rawUrl).hostname.toLowerCase();
  } catch {
    return '';
  }
}
