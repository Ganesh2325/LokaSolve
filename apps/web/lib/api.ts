export class ApiError extends Error {
  status: number;
  code: string;
  details: { field: string; message: string }[];

  constructor(status: number, code: string, message: string, details: { field: string; message: string }[] = []) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export type User = {
  id: string;
  email: string | null;
  phone_e164: string | null;
  display_name: string;
  preferred_language: string;
  status: string;
  is_demo: boolean;
  roles: string[];
  permissions?: string[];
  notification_preferences: Record<string, boolean>;
  privacy_preferences: Record<string, boolean>;
  email_verified: boolean;
  phone_verified: boolean;
};

export type Session = {
  user: User;
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
};

export type Organization = {
  id: string;
  name: string;
  org_type: string;
  status: string;
  is_demo: boolean;
  member_role?: string;
};

export type SavedPlace = {
  id: string;
  label: string | null;
  address_text: string | null;
  locality_text: string | null;
  latitude: number | null;
  longitude: number | null;
  source: string;
};

type RequestOptions = {
  method?: string;
  token?: string;
  body?: unknown;
};

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers();
  headers.set("X-LokaSolve-Client", "web");
  if (options.body !== undefined) headers.set("Content-Type", "application/json");
  if (options.token) headers.set("Authorization", `Bearer ${options.token}`);
  const response = await fetch(path, {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    credentials: "include",
  });
  if (response.status === 204) return undefined as T;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = payload.error ?? {};
    throw new ApiError(response.status, error.code ?? "request_failed", error.message ?? "Request failed.", error.details ?? []);
  }
  return payload as T;
}

let refreshInFlight: Promise<Session | null> | null = null;

export async function refreshSession(): Promise<Session | null> {
  if (!refreshInFlight) {
    refreshInFlight = fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-LokaSolve-Client": "web" },
      body: "{}",
    })
      .then(async (response) => {
        if (!response.ok) return null;
        return (await response.json()) as Session;
      })
      .finally(() => {
        refreshInFlight = null;
      });
  }
  return refreshInFlight;
}
