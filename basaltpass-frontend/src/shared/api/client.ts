import axios from 'axios'
import { clearAllAccessTokens, clearAllScopeCookies, clearAccessToken, getAuthScope, getCSRFCookie, setAccessToken } from '../utils/auth'
import { setSessionNotice } from '../utils/sessionNotice'
import { getApiBase, getApiTimeoutMs, getConsoleUserUrl } from '../config/env'

const client = axios.create({
  baseURL: getApiBase(),
  withCredentials: true,
  timeout: getApiTimeoutMs(),
})

const isAuthEntryPath = (pathname: string) => {
  return pathname === '/login'
    || pathname === '/register'
    || /^\/auth\/tenant\/[^/]+\/(login|register)$/.test(pathname)
    || /^\/tenant\/[^/]+\/(login|register)$/.test(pathname)
}

const buildSessionExpiredRedirect = () => {
  if (typeof window !== 'undefined') {
    const params = new URLSearchParams(window.location.search || '')
    const tenantCode = (params.get('tenant') || params.get('tenant_code') || '').trim()
    if (tenantCode) {
      const redirectTarget = `${window.location.pathname}${window.location.search || ''}`
      return `/auth/tenant/${encodeURIComponent(tenantCode)}/login?redirect=${encodeURIComponent(redirectTarget)}`
    }
  }

  const scope = getAuthScope()
  const userConsoleUrl = getConsoleUserUrl().replace(/\/+$/, '')

  if (scope === 'tenant' || scope === 'admin') {
    if (userConsoleUrl) {
      return `${userConsoleUrl}/login`
    }
    return '/'
  }

  return '/login'
}

// isnotranslatedtokentranslated
let isRefreshing = false
// translatedrequesttranslated
let failedQueue: Array<{
  resolve: (value?: any) => void
  reject: (error?: any) => void
}> = []

// translatedrequest
const processQueue = (error: any, token: string | null = null) => {
  failedQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error)
    } else {
      resolve(token)
    }
  })
  
  failedQueue = []
}

client.interceptors.request.use((config) => {
  // Scope header: user/tenant/admin (defaults to user)
  config.headers = config.headers || {}
  const scope = getAuthScope()
  ;(config.headers as any)['X-Auth-Scope'] = scope
  const csrfToken = getCSRFCookie()
  if (csrfToken) {
    ;(config.headers as any)['X-CSRF-Token'] = csrfToken
  }
  
  return config
})

// responsetranslated - translated401errorandtranslatedtoken
client.interceptors.response.use(
  (response) => {
    return response
  },
  async (error) => {
    const originalRequest = error.config
    const pathname = typeof window !== 'undefined' ? (window.location.pathname || '') : ''

    if (error.response?.status === 401 && !originalRequest._retry) {
      const url: string = originalRequest?.url || ''
      const shouldSkipRefreshOnAuthPage = isAuthEntryPath(pathname)
      // translated，translatederrortranslated（translated/translatedlogin）
      const isAuthEndpoint = url.includes('/api/v1/auth/login')
        || url.includes('/api/v1/auth/verify-2fa')
        || url.includes('/api/v1/auth/refresh')
      if (isAuthEndpoint || shouldSkipRefreshOnAuthPage) {
        return Promise.reject(error)
      }
      if (isRefreshing) {
        // translatedtoken，translatedrequesttranslated
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject })
        }).then(() => {
          return client(originalRequest)
        }).catch(err => {
          return Promise.reject(err)
        })
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        let response
        let retries = 0
        const maxRetries = 3
        while (retries <= maxRetries) {
          try {
            response = await client.post('/api/v1/auth/refresh')
            break
          } catch (rErr: any) {
            retries++
            if (retries > maxRetries) {
              throw rErr
            }
            await new Promise(res => setTimeout(res, 1000 * retries))
          }
        }
        
        const { access_token } = response!.data
        
        // Keep the latest access token only in memory for same-page helpers.
        setAccessToken(access_token)
        
        // translatedrequest
        processQueue(null, null)
        
        // translatedrequest
        return client(originalRequest)
        
      } catch (refreshError) {
        // translatedfailed，translatedtokentranslated
        processQueue(refreshError, null)
        clearAccessToken()
        clearAllAccessTokens()
        clearAllScopeCookies()
        setSessionNotice('session_expired')
        
        // translatedistranslatedlogintranslated，translatedtologintranslated
        const redirectTarget = buildSessionExpiredRedirect()
        if (window.location.pathname + window.location.search !== redirectTarget) {
          window.location.href = redirectTarget
        }
        
        return Promise.reject(refreshError)
      } finally {
        isRefreshing = false
      }
    }
    
    return Promise.reject(error)
  }
)

export default client 
