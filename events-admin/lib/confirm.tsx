'use client';
import { createContext, useContext, useState, ReactNode, useCallback } from 'react';

interface ConfirmState { title: string; msg: string; cb: () => Promise<void>; }
const Ctx = createContext<(title: string, msg: string, cb: () => Promise<void>) => void>(() => {});
export const useConfirm = () => useContext(Ctx);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ConfirmState | null>(null);
  const [running, setRunning] = useState(false);

  const openConfirm = useCallback((title: string, msg: string, cb: () => Promise<void>) => {
    setState({ title, msg, cb });
  }, []);

  async function handleOk() {
    if (!state) return;
    setRunning(true);
    try { await state.cb(); } finally { setRunning(false); setState(null); }
  }

  return (
    <Ctx.Provider value={openConfirm}>
      {children}
      {state && (
        <div className="modal-overlay" onClick={(e) => { if (e.target === e.currentTarget) setState(null); }}>
          <div className="modal modal-sm">
            <div className="modal-header">
              <div className="modal-title">{state.title}</div>
              <button className="modal-close" onClick={() => setState(null)}>✕</button>
            </div>
            <div className="modal-body">
              <p style={{ color: 'var(--text-2)', fontSize: '.9rem' }}>{state.msg}</p>
            </div>
            <div className="modal-footer">
              <button className="btn" onClick={() => setState(null)}>Cancel</button>
              <button className="btn btn-danger" onClick={handleOk} disabled={running}>
                {running ? 'Working…' : 'Confirm'}
              </button>
            </div>
          </div>
        </div>
      )}
    </Ctx.Provider>
  );
}