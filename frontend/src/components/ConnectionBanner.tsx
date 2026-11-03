import { useSocket } from '../context/SocketProvider';

export default function ConnectionBanner() {
  const { client, state, retry } = useSocket();
  if (!client || state === 'open') return null;
  const label = state === 'reconnecting' ? 'Reconnecting…' : state === 'closed' ? 'Disconnected' : 'Connecting…';
  return (
    <div role="status" className="fixed left-1/2 top-2 z-[10001] flex -translate-x-1/2 items-center gap-3 rounded-full bg-amber-100 px-4 py-2 text-sm text-amber-900 shadow">
      <span>{label}</span>
      {state !== 'connecting' && (
        <button type="button" className="rounded-full bg-amber-900 px-3 py-1 text-xs text-white" onClick={retry}>
          Retry
        </button>
      )}
    </div>
  );
}
