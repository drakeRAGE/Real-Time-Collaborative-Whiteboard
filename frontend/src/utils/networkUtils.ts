export function isOnline(): boolean {
  return navigator.onLine;
}

export function updateStatus(
  status: boolean,
  setIsOnline: (online: boolean) => void,
  setShowBackOnline: (show: boolean) => void,
  onlineHash: string,
  offlineHash: string,
): void {
  setIsOnline(status);
  localStorage.setItem('network', status ? onlineHash : offlineHash);
  if (status) {
    setShowBackOnline(true);
    setTimeout(() => setShowBackOnline(false), 3000);
  }
}
