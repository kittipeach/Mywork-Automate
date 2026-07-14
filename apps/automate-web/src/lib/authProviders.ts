import { z } from 'zod';

// Mirrors GET /api/automate/v1/auth/config (docs/spec/06 §Admin & Auth).
// The frontend uses this to decide whether to render the local-login form —
// it must never guess; the server is the source of truth for the local-auth guard.
export const authConfigSchema = z.object({
  providers: z.array(z.enum(['entra', 'local'])).min(1),
});

export type AuthConfig = z.infer<typeof authConfigSchema>;

/** Parse and validate the /auth/config payload; throws on malformed input. */
export function parseAuthConfig(raw: unknown): AuthConfig {
  return authConfigSchema.parse(raw);
}

/** Whether the local-login form should be shown. */
export function shouldShowLocalLogin(cfg: AuthConfig): boolean {
  return cfg.providers.includes('local');
}

/** Whether Entra SSO is offered (always expected in every environment). */
export function hasEntraSso(cfg: AuthConfig): boolean {
  return cfg.providers.includes('entra');
}
