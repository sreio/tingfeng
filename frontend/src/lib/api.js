let csrf = '';
let onUnauthorized = () => {};
export function configureAuth(token, onExpired) {
  csrf = token || '';
  if (onExpired) onUnauthorized = onExpired;
}
export class APIError extends Error {
  constructor(message, status, data) {
    super(message);
    this.status = status;
    this.data = data;
  }
}
export async function api(path, { method = 'GET', body, signal } = {}) {
  let response;
  try {
    response = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      signal,
      headers: {
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
        ...(method !== 'GET' && csrf ? { 'X-CSRF-Token': csrf } : {})
      },
      body: body !== undefined ? JSON.stringify(body) : undefined
    });
  } catch (e) {
    if (e.name === 'AbortError') throw e;
    throw new APIError('无法连接服务器，请检查网络后重试。', 0);
  }
  let data;
  try {
    data = await response.json();
  } catch {
    throw new APIError('服务器没有返回有效结果。', response.status);
  }
  if (!response.ok) {
    if (response.status === 401 && !path.startsWith('/auth/')) onUnauthorized();
    throw new APIError(data.error || '操作失败', response.status, data);
  }
  return data;
}
export async function download(path, name) {
  const response = await fetch('/api' + path, { credentials: 'same-origin' });
  if (!response.ok) throw new APIError('下载失败，请刷新页面后重试。', response.status);
  const blob = await response.blob();
  saveFile(blob, name);
}
export function saveFile(blob, name) {
  const url = URL.createObjectURL(blob),
    a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}
