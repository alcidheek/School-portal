import { useEffect, useState } from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { resultRequest } from '../services/resultService.ts';

export default function RoleGuard({ role }: { role: 'ADMIN' | 'TEACHER' | 'STUDENT' }) {
  const [state, setState] = useState('loading');
  useEffect(() => {
    let active = true;
    resultRequest<{ role: string }>('me').then((account) => {
      if (active) setState(account.role === role ? 'allowed' : 'forbidden');
    }).catch(() => { if (active) setState('forbidden'); });
    return () => { active = false; };
  }, [role]);
  if (state === 'loading') return <p role="status">Checking your account…</p>;
  if (state !== 'allowed') return <Navigate to="/" replace />;
  return <Outlet />;
}
