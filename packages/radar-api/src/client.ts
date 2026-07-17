export type ResponseDecoder<Result> = (value: unknown) => Result;
export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

export interface RadarApiClientOptions {
  baseUrl: string;
  fetch: FetchLike;
  getAccessToken: () => Promise<string | null> | string | null;
}

export interface RadarRequestOptions<Result> extends Omit<RequestInit, 'headers'> {
  decode?: ResponseDecoder<Result>;
  headers?: HeadersInit;
}

export interface RadarApiClient {
  request<Result>(path: string, init?: RadarRequestOptions<Result>): Promise<Result>;
}

export class RadarApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly requestId: string | null,
  ) {
    super(message);
    this.name = 'RadarApiError';
  }
}

function endpoint(baseUrl: string, path: string) {
  return `${baseUrl.replace(/\/$/, '')}/${path.replace(/^\//, '')}`;
}

async function errorDetail(response: Response) {
  try {
    const body = (await response.json()) as { detail?: unknown };
    if (typeof body.detail === 'string') return body.detail;
  } catch {
    // Preserve the HTTP fallback for empty and non-JSON error bodies.
  }
  return `API request failed with ${response.status}`;
}

export function createRadarApiClient(options: RadarApiClientOptions): RadarApiClient {
  return {
    async request<Result>(path: string, init: RadarRequestOptions<Result> = {}): Promise<Result> {
      const token = await options.getAccessToken();
      const headers = new Headers(init.headers);
      headers.set('Accept', 'application/json');
      if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
      if (token && !headers.has('Authorization')) headers.set('Authorization', `Bearer ${token}`);

      const response = await options.fetch(endpoint(options.baseUrl, path), {
        ...init,
        headers,
        cache: 'no-store',
      });
      if (!response.ok) {
        throw new RadarApiError(
          await errorDetail(response),
          response.status,
          response.headers.get('x-request-id'),
        );
      }
      if (response.status === 204) return undefined as Result;

      const value: unknown = await response.json();
      return init.decode ? init.decode(value) : (value as Result);
    },
  };
}

export function idempotencyHeaders(key: string) {
  if (key.length < 8 || key.length > 128) {
    throw new Error('Idempotency key must contain 8 to 128 characters');
  }
  return { 'Idempotency-Key': key };
}
