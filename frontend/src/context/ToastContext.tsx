import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';

interface ToastItem {
  id: number;
  message: string;
}

const ToastContext = createContext<((message: string) => void) | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string) => {
    const id = Date.now() + Math.floor(Math.random() * 1000);
    setToasts((prev) => [...prev, { id, message }].slice(-4));
    setTimeout(() => {
      setToasts((prev) => prev.filter((item) => item.id !== id));
    }, 3500);
  }, []);

  const value = useMemo(() => toast, [toast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="fixed bottom-4 right-4 z-[10000] flex w-80 max-w-[90vw] flex-col gap-2" aria-live="polite">
        {toasts.map((item) => (
          <div key={item.id} role="status" className="rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm text-gray-800 shadow-lg">
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): (message: string) => void {
  const toast = useContext(ToastContext);
  if (!toast) throw new Error('useToast requires ToastProvider');
  return toast;
}
