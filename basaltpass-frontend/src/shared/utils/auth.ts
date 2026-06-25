export type ConsoleScope = 'user' | 'tenant' | 'admin'

export function getAuthScope(): ConsoleScope {
  const envScope = (import.meta as any).env?.VITE_AUTH_SCOPE
  if (envScope === 'user' || envScope === 'tenant' || envScope === 'admin') {
    return envScope
  }

  if (typeof window !== 'undefined') {
    const path = window.location.pathname || '/'
    if (path.startsWith('/admin')) return 'admin'
    if (path.startsWith('/tenant')) return 'tenant'
  }

  return 'user'
}

export function getTokenKey(): string {
  return getTokenKeyForScope(getAuthScope())
}

export function getTokenKeyForScope(scope: ConsoleScope): string {
  const envTokenKey = (import.meta as any).env?.VITE_TOKEN_KEY
  if (envTokenKey && scope === getAuthScope()) {
    return envTokenKey
  }

  switch (scope) {
    case 'admin':
      return 'bp_admin_access_token'
    case 'tenant':
      return 'bp_tenant_access_token'
    default:
      return 'bp_user_access_token'
  }
}

let accessTokenMemory: string | null = null

export function setAccessToken(token: string) {
  accessTokenMemory = token || null
}

export function getAccessToken(): string | null {
  return accessTokenMemory
}

export function clearAccessToken() {
  accessTokenMemory = null
}

export function clearAccessTokenForScope(scope: ConsoleScope) {
  localStorage.removeItem(getTokenKeyForScope(scope))
}

export function clearAllAccessTokens() {
  clearAccessTokenForScope('user')
  clearAccessTokenForScope('tenant')
  clearAccessTokenForScope('admin')
}

function expireCookie(name: string) {
  if (typeof document === 'undefined') {
    return
  }

  document.cookie = `${name}=; Max-Age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/; SameSite=Lax`
}

export function getCSRFCookie(): string {
  if (typeof document === 'undefined') {
    return ''
  }

  const scope = getAuthScope()
  const names = scope === 'user' ? ['csrf_token'] : [`csrf_token_${scope}`, 'csrf_token']
  const cookies = document.cookie.split(';').map((part) => part.trim())
  for (const name of names) {
    const prefix = `${name}=`
    const found = cookies.find((cookie) => cookie.startsWith(prefix))
    if (found) {
      return decodeURIComponent(found.slice(prefix.length))
    }
  }
  return ''
}

export function clearScopeCookies(scope: ConsoleScope) {
  if (scope === 'user') {
    expireCookie('access_token')
    expireCookie('refresh_token')
    expireCookie('csrf_token')
    return
  }

  expireCookie(`access_token_${scope}`)
  expireCookie(`refresh_token_${scope}`)
  expireCookie(`csrf_token_${scope}`)
}

export function clearAllScopeCookies() {
  clearScopeCookies('user')
  clearScopeCookies('tenant')
  clearScopeCookies('admin')
}
