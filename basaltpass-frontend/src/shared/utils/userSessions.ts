import { decodeJWT } from './jwt'

const USER_SESSION_STORAGE_KEY = 'bp_user_console_sessions'

export interface UserConsoleSession {
  key: string
  user_id: number
  tenant_id: number
  tenant_code?: string
  tenant_memberships?: Array<{
    id: number
    name?: string
    code?: string
    role?: string
  }>
  email: string
  nickname?: string
  avatar_url?: string
  tenant_role?: string
  tenant_name?: string
  is_super_admin?: boolean
  token?: string
  last_used_at: string
}

interface SessionProfileLike {
  id: number
  email: string
  nickname?: string
  avatar_url?: string
  is_super_admin?: boolean
  tenant_id?: number
  tenant_role?: string
}

interface SessionTenantLike {
  id: number
  name?: string
  code?: string
  role?: string
  metadata?: Record<string, any>
}

function isBrowser() {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function buildSessionKey(userID: number, tenantID: number) {
  return `${userID}:${tenantID}`
}

function getTokenExpiryMs(token: string) {
  const decoded = decodeJWT(token)
  const exp = Number(decoded?.exp || 0)
  if (!exp) {
    return 0
  }
  return exp * 1000
}

function readSessions(): UserConsoleSession[] {
  if (!isBrowser()) {
    return []
  }

  try {
    const raw = window.localStorage.getItem(USER_SESSION_STORAGE_KEY)
    if (!raw) {
      return []
    }

    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }

    let strippedSensitiveToken = false
    const sessions = parsed.filter((item): item is UserConsoleSession => {
      return !!item && typeof item === 'object' && typeof item.key === 'string'
    }).map((item) => {
      if ('token' in item) {
        strippedSensitiveToken = true
      }
      const { token, ...session } = item
      void token
      return session as UserConsoleSession
    })
    if (strippedSensitiveToken) {
      writeSessions(sessions)
    }
    return sessions
  } catch {
    return []
  }
}

function writeSessions(sessions: UserConsoleSession[]) {
  if (!isBrowser()) {
    return
  }

  window.localStorage.setItem(USER_SESSION_STORAGE_KEY, JSON.stringify(sessions))
}

export function listUserConsoleSessions() {
  return readSessions().sort((a, b) => new Date(b.last_used_at).getTime() - new Date(a.last_used_at).getTime())
}

export function isUserConsoleSessionExpired(session: UserConsoleSession, leewayMs = 30_000) {
  if (!session.token) {
    return false
  }
  const expiresAt = getTokenExpiryMs(session.token)
  if (!expiresAt) {
    return true
  }
  return expiresAt <= Date.now() + leewayMs
}

export function pruneExpiredUserConsoleSessions() {
  const sessions = readSessions()
  const activeSessions = sessions.filter((session) => !isUserConsoleSessionExpired(session))

  if (activeSessions.length !== sessions.length) {
    writeSessions(activeSessions)
  }

  return activeSessions.sort((a, b) => new Date(b.last_used_at).getTime() - new Date(a.last_used_at).getTime())
}

export function upsertUserConsoleSession(
  profile: SessionProfileLike,
  token?: string,
  tenants: SessionTenantLike[] = [],
) {
  if (!profile?.id || !profile?.email) {
    return
  }

  const tenantID = Number(profile.tenant_id || 0)
  const key = buildSessionKey(profile.id, tenantID)
  const currentTenant = tenantID > 0 ? tenants.find((tenant) => Number(tenant.id) === tenantID) : null
  const tenantMemberships = tenants
    .map((tenant) => ({
      id: Number(tenant.id || 0),
      name: tenant.name,
      code: tenant.code,
      role: String(tenant.metadata?.user_role || tenant.role || '').toLowerCase(),
    }))
    .filter((tenant) => tenant.id > 0)
  const sessions = readSessions().filter((session) => session.key !== key)

  sessions.unshift({
    key,
    user_id: profile.id,
    tenant_id: tenantID,
    tenant_code: currentTenant?.code,
    tenant_memberships: tenantMemberships,
    email: profile.email,
    nickname: profile.nickname,
    avatar_url: profile.avatar_url,
    tenant_role: profile.tenant_role,
    tenant_name: currentTenant?.name || (tenantID === 0 ? 'translated' : undefined),
    is_super_admin: !!profile.is_super_admin,
    ...(token ? { token } : {}),
    last_used_at: new Date().toISOString(),
  })

  writeSessions(sessions)
}

export function removeUserConsoleSession(userID: number, tenantID: number) {
  const key = buildSessionKey(userID, tenantID)
  writeSessions(readSessions().filter((session) => session.key !== key))
}

export function removeUserConsoleSessionByKey(sessionKey: string) {
  writeSessions(readSessions().filter((session) => session.key !== sessionKey))
}

export function updateStoredUserSessionToken(token: string) {
  void token
}

export function getUserConsoleSession(sessionKey: string) {
  return readSessions().find((session) => session.key === sessionKey) || null
}
