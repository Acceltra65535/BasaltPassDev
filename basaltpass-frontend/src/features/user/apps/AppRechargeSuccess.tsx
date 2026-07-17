import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CheckCircleIcon, ExclamationTriangleIcon } from '@heroicons/react/24/solid'
import Layout from '@features/user/components/Layout'
import { paymentAPI, type PaymentSession } from '@api/subscription/payment/payment'
import { ROUTES } from '@constants'
import { useI18n } from '@shared/i18n'
import { PAlert, PButton, PCard, PSkeleton } from '@ui'

const REDIRECT_DELAY_SECONDS = 10

const getReturnURL = (raw: string | null) => {
  if (!raw) return ''
  try {
    const url = new URL(raw)
    if (url.protocol !== 'https:' && url.protocol !== 'http:') return ''
    return url.toString()
  } catch {
    return ''
  }
}

const getReturnHost = (raw: string) => {
  try {
    return new URL(raw).host
  } catch {
    return ''
  }
}

const getAppName = (session: PaymentSession | null) => {
  const metadata = session?.PaymentIntent?.Metadata
  if (!metadata) return ''
  try {
    const parsed = JSON.parse(metadata)
    return typeof parsed.app_name === 'string' ? parsed.app_name : ''
  } catch {
    return ''
  }
}

export default function AppRechargeSuccess() {
  const { t } = useI18n()
  const [searchParams] = useSearchParams()
  const sessionId = searchParams.get('session_id') || ''
  const returnURL = useMemo(() => getReturnURL(searchParams.get('return_url')), [searchParams])
  const returnHost = useMemo(() => getReturnHost(returnURL), [returnURL])
  const [session, setSession] = useState<PaymentSession | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [countdown, setCountdown] = useState(REDIRECT_DELAY_SECONDS)

  useEffect(() => {
    let active = true
    const reconcile = async () => {
      if (!sessionId) {
        setError(t('pages.appRecharge.success.errors.missingSession'))
        setLoading(false)
        return
      }
      setLoading(true)
      setError('')
      try {
        const response = await paymentAPI.reconcileWalletTopUpSession(sessionId)
        if (!active) return
        setSession(response.session)
        if (response.session.Status !== 'complete') {
          setError(t('pages.appRecharge.success.errors.notComplete'))
        }
      } catch (e: any) {
        if (active) setError(e.response?.data?.error || t('pages.appRecharge.success.errors.reconcileFailed'))
      } finally {
        if (active) setLoading(false)
      }
    }

    void reconcile()
    return () => {
      active = false
    }
  }, [sessionId, t])

  useEffect(() => {
    if (loading || error || !returnURL || session?.Status !== 'complete') return
    setCountdown(REDIRECT_DELAY_SECONDS)
    const interval = window.setInterval(() => {
      setCountdown((value) => {
        if (value <= 1) {
          window.clearInterval(interval)
          window.location.href = returnURL
          return 0
        }
        return value - 1
      })
    }, 1000)

    return () => window.clearInterval(interval)
  }, [error, loading, returnURL, session?.Status])

  const appName = getAppName(session)

  return (
    <Layout>
      <div className="mx-auto max-w-2xl py-6">
        <PCard padding="lg">
          {loading ? (
            <PSkeleton.Content cards={1} />
          ) : error ? (
            <div className="space-y-6 text-center">
              <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-amber-100">
                <ExclamationTriangleIcon className="h-9 w-9 text-amber-600" />
              </div>
              <div className="space-y-2">
                <h1 className="text-2xl font-semibold text-gray-900">{t('pages.appRecharge.success.pendingTitle')}</h1>
                <p className="text-sm text-gray-600">{t('pages.appRecharge.success.pendingDescription')}</p>
              </div>
              <PAlert variant="warning" message={error} />
              <div className="flex flex-col gap-3 sm:flex-row sm:justify-center">
                <PButton type="button" onClick={() => window.location.reload()}>
                  {t('pages.appRecharge.success.actions.retry')}
                </PButton>
                <Link to={ROUTES.user.wallet}>
                  <PButton type="button" variant="secondary">
                    {t('pages.appRecharge.success.actions.wallet')}
                  </PButton>
                </Link>
              </div>
            </div>
          ) : (
            <div className="space-y-6 text-center">
              <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-green-100">
                <CheckCircleIcon className="h-10 w-10 text-green-600" />
              </div>
              <div className="space-y-2">
                <h1 className="text-2xl font-semibold text-gray-900">{t('pages.appRecharge.success.title')}</h1>
                <p className="text-sm text-gray-600">
                  {t('pages.appRecharge.success.description', { app: appName || t('pages.appRecharge.header.appFallback') })}
                </p>
              </div>

              {returnURL ? (
                <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-900">
                  {t('pages.appRecharge.success.redirecting', { seconds: countdown, host: returnHost })}
                </div>
              ) : (
                <div className="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700">
                  {t('pages.appRecharge.success.noReturnUrl')}
                </div>
              )}

              <div className="flex flex-col gap-3 sm:flex-row sm:justify-center">
                {returnURL && (
                  <PButton type="button" onClick={() => { window.location.href = returnURL }}>
                    {t('pages.appRecharge.success.actions.returnNow')}
                  </PButton>
                )}
                <Link to={ROUTES.user.wallet}>
                  <PButton type="button" variant={returnURL ? 'secondary' : 'primary'}>
                    {t('pages.appRecharge.success.actions.wallet')}
                  </PButton>
                </Link>
              </div>
            </div>
          )}
        </PCard>
      </div>
    </Layout>
  )
}
