'use client';

import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { Loader2, Lock, LogIn, AlertCircle } from 'lucide-react';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { getJSON } from '@/api/hooks';
import { useLogin } from '@/api/auth';
import { ApiError } from '@/api/mutations';
import { parseAuthConfig, shouldShowLocalLogin } from '@/lib/authProviders';

const loginSchema = z.object({
  email: z.string().min(1, 'Email is required').email('Enter a valid email'),
  password: z.string().min(1, 'Password is required'),
});
type LoginForm = z.infer<typeof loginSchema>;

// Entra SSO placeholder — the real redirect is wired by the platform team. Kept
// as a stub link so the button is always present (every environment offers SSO).
const ENTRA_SSO_URL = '/api/automate/v1/auth/entra/login';

/** Map a login ApiError to a user-facing message. */
function loginErrorMessage(error: unknown): string | null {
  if (error instanceof ApiError) {
    if (error.status === 401) return 'Invalid email or password.';
    if (error.status === 423) return 'This account is locked. Please try again later.';
    return 'Sign-in failed. Please try again.';
  }
  return error ? 'Sign-in failed. Please try again.' : null;
}

export default function LoginPage() {
  const router = useRouter();
  const login = useLogin();

  const configQuery = useQuery({
    queryKey: ['authConfig'],
    queryFn: async () => parseAuthConfig(await getJSON('/auth/config')),
    staleTime: Infinity,
    retry: false,
  });
  const showLocal = configQuery.data ? shouldShowLocalLogin(configQuery.data) : false;

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({ resolver: zodResolver(loginSchema) });

  const onSubmit = (values: LoginForm) => {
    login.mutate(values, {
      onSuccess: () => router.push('/automate'),
    });
  };

  const errorMsg = loginErrorMessage(login.error);

  return (
    <main className="grid min-h-screen place-items-center bg-surface-sunken p-6">
      <Card className="w-full max-w-sm p-8">
        <div className="mb-6 flex flex-col items-center text-center">
          <div className="mb-3 grid h-11 w-11 place-items-center rounded-xl bg-brand text-brand-fg">
            <Lock className="h-5 w-5" aria-hidden />
          </div>
          <h1 className="text-lg font-semibold text-ink">Sign in to MyWork Automate</h1>
          <p className="mt-1 text-sm text-ink-muted">Workflow automation for HR operations</p>
        </div>

        {configQuery.isLoading && (
          <div className="flex items-center justify-center gap-2 py-6 text-sm text-ink-muted">
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden /> Loading sign-in options…
          </div>
        )}

        {showLocal && (
          <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
            <div>
              <label htmlFor="email" className="mb-1 block text-sm font-medium text-ink">
                Email
              </label>
              <input
                id="email"
                type="email"
                autoComplete="username"
                aria-invalid={!!errors.email}
                {...register('email')}
                className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
              />
              {errors.email && (
                <p role="alert" className="mt-1 text-xs text-danger">
                  {errors.email.message}
                </p>
              )}
            </div>

            <div>
              <label htmlFor="password" className="mb-1 block text-sm font-medium text-ink">
                Password
              </label>
              <input
                id="password"
                type="password"
                autoComplete="current-password"
                aria-invalid={!!errors.password}
                {...register('password')}
                className="h-9 w-full rounded-md border border-border bg-surface px-3 text-sm outline-none focus:ring-2 focus:ring-brand"
              />
              {errors.password && (
                <p role="alert" className="mt-1 text-xs text-danger">
                  {errors.password.message}
                </p>
              )}
            </div>

            {errorMsg && (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-md bg-danger/10 px-3 py-2 text-sm text-danger"
              >
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                <span>{errorMsg}</span>
              </div>
            )}

            <Button type="submit" className="w-full" disabled={login.isPending}>
              {login.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
              ) : (
                <LogIn className="h-4 w-4" aria-hidden />
              )}
              {login.isPending ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        )}

        {showLocal && (
          <div className="my-5 flex items-center gap-3 text-xs text-ink-subtle">
            <span className="h-px flex-1 bg-border" />
            or
            <span className="h-px flex-1 bg-border" />
          </div>
        )}

        <Button
          type="button"
          variant="secondary"
          className="w-full"
          onClick={() => {
            window.location.href = ENTRA_SSO_URL;
          }}
        >
          Continue with Microsoft Entra
        </Button>
      </Card>
    </main>
  );
}
