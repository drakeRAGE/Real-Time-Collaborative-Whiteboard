import type { FallbackProps } from 'react-error-boundary';

export default function RouteFallback({ error, resetErrorBoundary }: FallbackProps) {
  const message = error instanceof Error ? error.message : 'Something went wrong';
  return (
    <div role="alert" className="m-6 rounded-lg border border-red-200 bg-red-50 p-4 text-red-800">
      <p className="font-medium">This page hit an error.</p>
      <p className="mt-1 text-sm">{message}</p>
      <button type="button" className="mt-3 rounded-md bg-red-700 px-3 py-1.5 text-sm text-white" onClick={resetErrorBoundary}>
        Try again
      </button>
    </div>
  );
}
