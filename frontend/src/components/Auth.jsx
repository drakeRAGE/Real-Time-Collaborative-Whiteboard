import { useState } from 'react';
import { supabase } from '../utils/supabase';
import { useNavigate } from 'react-router-dom';

export function Auth() {
  const [loading, setLoading] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const navigate = useNavigate();

  const handleSignUp = async (e) => {
    e.preventDefault();
    try {
      setLoading(true);
      const { error } = await supabase.auth.signUp({ email, password });
      if (error) throw error;
      alert('Check your email for the confirmation link!');
    } catch (error) {
      alert(error.message);
    } finally {
      setLoading(false);
    }
  };

  const handleSignIn = async (e) => {
    e.preventDefault();
    try {
      setLoading(true);
      const { error } = await supabase.auth.signInWithPassword({ email, password });
      if (error) throw error;
      navigate('/');
    } catch (error) {
      alert(error.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-900 via-gray-800 to-black p-4">
      <div className="bg-white/10 backdrop-blur-lg border border-white/20 rounded-2xl shadow-xl w-full max-w-md p-8">
        <h1 className="text-3xl font-bold text-white text-center mb-6">Welcome Back</h1>
        <p className="text-gray-300 text-center mb-8">
          Sign in to your account or create a new one.
        </p>

        <form className="space-y-5">
          <div>
            <label className="block text-gray-200 text-sm mb-1">Email Address</label>
            <input
              type="email"
              placeholder="dragbos@example.com"
              value={email}
              required
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-4 py-2 rounded-lg bg-gray-900/60 text-white placeholder-gray-400 border border-gray-700 focus:ring-2 focus:ring-indigo-500 outline-none"
            />
          </div>

          <div>
            <label className="block text-gray-200 text-sm mb-1">Password</label>
            <input
              type="password"
              placeholder="••••••••"
              value={password}
              required
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-4 py-2 rounded-lg bg-gray-900/60 text-white placeholder-gray-400 border border-gray-700 focus:ring-2 focus:ring-indigo-500 outline-none"
            />
          </div>

          <div className="flex gap-4">
            <button
              onClick={handleSignIn}
              disabled={loading}
              className="flex-1 bg-indigo-600 hover:bg-indigo-700 text-white font-semibold py-2 rounded-lg transition-all duration-300 disabled:opacity-50"
            >
              {loading ? 'Signing In...' : 'Sign In'}
            </button>
            <button
              onClick={handleSignUp}
              disabled={loading}
              className="flex-1 bg-gray-700 hover:bg-gray-800 text-white font-semibold py-2 rounded-lg transition-all duration-300 disabled:opacity-50"
            >
              {loading ? 'Signing Up...' : 'Sign Up'}
            </button>
          </div>
        </form>

        <p className="mt-6 text-center text-gray-400 text-sm">
          By continuing, you agree to our{' '}
          <span className="text-indigo-400 hover:underline cursor-pointer">Terms</span> and{' '}
          <span className="text-indigo-400 hover:underline cursor-pointer">Privacy Policy</span>.
        </p>
      </div>
    </div>
  );
}
