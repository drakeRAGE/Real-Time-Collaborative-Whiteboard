/** HTTP gateway origin → `ws(s)://host/ws`. */
export function websocketUrl(httpBase: string): string {
  const url = new URL(httpBase);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.pathname = '/ws';
  url.search = '';
  url.hash = '';
  return url.toString();
}
