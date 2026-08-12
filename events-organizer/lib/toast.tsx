'use client';
import { createContext, useContext, useState, useCallback, ReactNode } from 'react';
import { CheckCircle, XCircle, Info } from '@phosphor-icons/react';

type ToastType = 'success' | 'error' | 'info';
interface Toast { id: number; msg: string; type: ToastType; }
const icons = {
  success: <CheckCircle size={17} weight="fill" />,
  error: <XCircle size={17} weight="fill" />,
  info: <Info size={17} weight="fill" />,
};

const Ctx = createContext<(msg: string, type?: ToastType) => void>(() => {});
export const useToast = () => useContext(Ctx);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const toast = useCallback((msg: string, type: ToastType = 'info') => {
    const id = Date.now();
    setToasts(p => [...p, { id, msg, type }]);
    setTimeout(() => setToasts(p => p.filter(t => t.id !== id)), 4500);
  }, []);

  return (
    <Ctx.Provider value={toast}>
      {children}
      <div id="toast-root">
        {toasts.map(t => (
          <div key={t.id} className={`toast ${t.type}`}>
            <span style={{ flexShrink: 0 }}>{icons[t.type]}</span> {t.msg}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}