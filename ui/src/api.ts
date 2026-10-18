import { QueryClient } from '@tanstack/react-query';

import { setOffline, unreachableMessage } from './connection';

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
      refetchOnWindowFocus: true,
    },
  },
});

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin' };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { 'Content-Type': 'application/json' };
  }
  let res: Response;
  try {
    res = await fetch(`/api${path}`, init);
  } catch {
    setOffline(true);
    throw new ApiError(unreachableMessage, 0);
  }
  // Moonshed answers errors with JSON; anything else came from something in between,
  // like a proxy that couldn't reach it.
  const fromMoonshed = res.ok || res.headers.get('Content-Type')?.includes('application/json');
  if (!fromMoonshed && res.status >= 500) {
    setOffline(true);
    throw new ApiError(unreachableMessage, 0);
  }
  setOffline(false);
  if (res.status === 401 && !path.startsWith('/auth/')) {
    queryClient.invalidateQueries({ queryKey: ['auth'] });
  }
  if (!res.ok) {
    const data = await res.json().catch(() => null);
    throw new ApiError(data?.error ?? res.statusText, res.status);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json();
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  delete: (path: string) => request<void>('DELETE', path),
};

// uploadBundle uses XHR because fetch can't report upload progress.
export function uploadBundle(appId: string, file: File, onProgress: (pct: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `/api/apps/${appId}/deploy`);
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress(Math.round((e.loaded / e.total) * 100));
    xhr.onload = () => {
      if (xhr.status < 300) {
        resolve();
        return;
      }
      let message = xhr.statusText;
      try {
        message = JSON.parse(xhr.responseText).error ?? message;
      } catch {
        if (xhr.status >= 500) message = unreachableMessage;
      }
      reject(new ApiError(message, xhr.status));
    };
    xhr.onerror = () => reject(new ApiError(unreachableMessage, 0));
    const form = new FormData();
    form.append('bundle', file);
    xhr.send(form);
  });
}
