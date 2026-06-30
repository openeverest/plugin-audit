// Authentication helpers — mirror the host's auth.ts so the plugin uses the
// same in-memory access token + cookie refresh flow. The plugin runs in its
// own module scope and cannot read the host's token state directly, so it
// maintains its own copy.

type AuthTokenRequest = {
  grant_type: 'refresh_token';
  refresh_token_delivery: 'cookie';
};

type AuthTokenResponse = {
  access_token: string;
  expires_in: number;
};

let accessToken: string | null = null;
let accessTokenExpiry: number | null = null; // epoch ms

export const setAccessToken = (token: string, expiresInSeconds: number) => {
  accessToken = token;
  accessTokenExpiry = Date.now() + expiresInSeconds * 1000;
};

export const getAccessToken = () => accessToken;

export const getAccessTokenExpiry = () => accessTokenExpiry;

export const clearAccessToken = () => {
  accessToken = null;
  accessTokenExpiry = null;
};

// Returns the credential for API requests: the in-memory access token for
// internal sessions, falling back to localStorage for OIDC sessions.
export const getAuthToken = (): string | null =>
  accessToken || localStorage.getItem('everestToken');

let refreshPromise: Promise<string | null> | null = null;

// Single-flight refresh: all concurrent callers share one request.
export const refreshSession = (): Promise<string | null> => {
  if (!refreshPromise) {
    const payload: AuthTokenRequest = {
      grant_type: 'refresh_token',
      refresh_token_delivery: 'cookie',
    };
    refreshPromise = fetch('/v1/auth/token', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
      .then(async (res) => {
        if (!res.ok) throw new Error(`refresh failed: ${res.status}`);
        const data = (await res.json()) as AuthTokenResponse;
        setAccessToken(data.access_token, data.expires_in);
        return data.access_token;
      })
      .catch(() => {
        clearAccessToken();
        return null;
      })
      .finally(() => {
        refreshPromise = null;
      });
  }
  return refreshPromise;
};
