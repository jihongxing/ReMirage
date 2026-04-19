import { useState, useEffect, useCallback } from 'react';

const API_BASE = import.meta.env.VITE_API_BASE || '/api';

/** 从 localStorage 获取 JWT token */
function getToken(): string | null {
  return localStorage.getItem('mirage_token');
}

/** 构建带认证的 headers */
function authHeaders(extra?: Record<string, string>): Record<string, string> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...extra,
  };
  const token = getToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

export function useApi<T>(url: string, interval = 5000) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const resp = await fetch(`${API_BASE}${url}`, {
        headers: authHeaders(),
      });
      if (resp.status === 401) {
        // Token 过期或无效，清除并跳转
        localStorage.removeItem('mirage_token');
        setError('认证已过期');
        return;
      }
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const json = await resp.json();
      setData(json);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  }, [url]);

  useEffect(() => {
    fetchData();
    const id = setInterval(fetchData, interval);
    return () => clearInterval(id);
  }, [fetchData, interval]);

  return { data, loading, error, refetch: fetchData };
}

export async function apiPost<T>(url: string, body: unknown): Promise<T> {
  const resp = await fetch(`${API_BASE}${url}`, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(body),
  });
  if (resp.status === 401) {
    localStorage.removeItem('mirage_token');
    throw new Error('认证已过期');
  }
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.json();
}

export async function apiGet<T>(url: string): Promise<T> {
  const resp = await fetch(`${API_BASE}${url}`, {
    headers: authHeaders(),
  });
  if (resp.status === 401) {
    localStorage.removeItem('mirage_token');
    throw new Error('认证已过期');
  }
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.json();
}
