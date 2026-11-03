import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useToast } from '../context/ToastContext';
import { supabase, supabaseConfigured } from '../utils/supabase';

export function Auth() {
  const [loading, setLoading] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const navigate = useNavigate();
  const toast = useToast();

  const handleSignUp = async (event: FormEvent) => {
    event.preventDefault();
    if (!supabase) {
      toast('Supabase is not configured');
      return;
    }
    try {
      setLoading(true);
      const { error } = await supabase.auth.signUp({ email, password });
      if (error) throw error;
      toast('Check your email for the confirmation link.');
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Sign up failed');
    } finally {
      setLoading(false);
    }
  };

  const handleSignIn = async (event: FormEvent) => {
    event.preventDefault();
    if (!supabase) {
      toast('Supabase is not configured');
      return;
    }
    try {
      setLoading(true);
      const { error } = await supabase.auth.signInWithPassword({ email, password });
      if (error) throw error;
      navigate('/');
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Sign in failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-gray-900 via-gray-800 to-black p-4">
      <div className="w-full max-w-md rounded-2xl border border-white/20 bg-white/10 p-8 shadow-xl backdrop-blur-lg">
        <h1 className="mb-6 text-center text-3xl font-bold text-white">Welcome Back</h1>
        <p className="mb-8 text-center text-gray-300">Sign in to your account or create a new one.</p>
        {!supabaseConfigured && (
          <p className="mb-4 text-center text-sm text-amber-200">Supabase URL and anon key are not set, so sign-in cannot run.</p>
        )}
        <form className="space-y-5" onSubmit={(event) => void handleSignIn(event)}>
          <div>
            <label htmlFor="auth-email" className="mb-1 block text-sm text-gray-200">Email Address</label>
            <input id="auth-email" type="email" required value={email} onChange={(event) => setEmail(event.target.value)} placeholder="dragbos@example.com" className="w-full rounded-lg border border-gray-700 bg-gray-900/60 px-4 py-2 text-white outline-none" />
          </div>
          <div>
            <label htmlFor="auth-password" className="mb-1 block text-sm text-gray-200">Password</label>
            <input id="auth-password" type="password" required value={password} onChange={(event) => setPassword(event.target.value)} placeholder="••••••••" className="w-full rounded-lg border border-gray-700 bg-gray-900/60 px-4 py-2 text-white outline-none" />
          </div>
          <div className="flex gap-4">
            <button type="submit" disabled={loading || !supabaseConfigured} className="flex-1 rounded-lg bg-indigo-600 py-2 font-semibold text-white disabled:opacity-50">
              {loading ? 'Signing In...' : 'Sign In'}
            </button>
            <button type="button" disabled={loading || !supabaseConfigured} onClick={(event) => void handleSignUp(event)} className="flex-1 rounded-lg bg-gray-700 py-2 font-semibold text-white disabled:opacity-50">
              {loading ? 'Signing Up...' : 'Sign Up'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
