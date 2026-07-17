import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { CreditCardIcon, WalletIcon } from '@heroicons/react/24/outline'
import Layout from '@features/user/components/Layout'
import { paymentAPI, PaymentSession } from '@api/subscription/payment/payment'
import { PAlert, PButton, PCard, PPageHeader } from '@ui'

const getSessionId = (session: PaymentSession | null) =>
  session ? (session.StripeSessionID || (session as any).stripe_session_id || '') : ''

const formatMinor = (amount: number, currency: string) => {
  const divisor = ['jpy', 'krw'].includes(currency.toLowerCase()) ? 1 : 100
  return (amount / divisor).toFixed(divisor === 1 ? 0 : 2)
}

export default function Cashier() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const sessionId = params.get('session') || ''
  const kind = params.get('kind') || 'checkout'
  const [session, setSession] = useState<PaymentSession | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    const load = async () => {
      if (!sessionId) {
        setError('Missing checkout session')
        setLoading(false)
        return
      }
      try {
        const data = await paymentAPI.getPaymentSession(sessionId)
        if (active) setSession(data)
      } catch (e: any) {
        if (active) setError(e.response?.data?.error || e.message || 'Unable to load checkout session')
      } finally {
        if (active) setLoading(false)
      }
    }
    load()
    return () => {
      active = false
    }
  }, [sessionId])

  const title = useMemo(() => {
    if (kind === 'top_up') return 'Wallet top-up'
    if (kind === 'subscription') return 'Subscription checkout'
    return 'Checkout'
  }, [kind])

  const payWithStripe = () => {
    const id = getSessionId(session)
    if (!id) return
    window.location.href = paymentAPI.getPaymentCheckoutUrl(id)
  }

  return (
    <Layout>
      <div className="mx-auto max-w-3xl space-y-6">
        <PPageHeader title={title} description="Choose a payment method to continue." />

        {error && <PAlert variant="error" title="Checkout unavailable" message={error} />}

        <PCard padding="lg">
          {loading ? (
            <div className="py-10 text-center text-sm text-gray-500">Loading checkout...</div>
          ) : session ? (
            <div className="space-y-6">
              <div className="flex items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 p-4">
                <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-indigo-100">
                  {kind === 'top_up' ? <WalletIcon className="h-6 w-6 text-indigo-600" /> : <CreditCardIcon className="h-6 w-6 text-indigo-600" />}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-sm text-gray-500">Amount due</div>
                  <div className="text-2xl font-semibold text-gray-900">
                    {formatMinor(session.Amount || 0, session.Currency || 'USD')} {session.Currency}
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                <button
                  type="button"
                  onClick={payWithStripe}
                  className="flex w-full items-center justify-between rounded-lg border border-indigo-300 bg-white p-4 text-left shadow-sm transition hover:border-indigo-500 hover:bg-indigo-50"
                >
                  <span>
                    <span className="block text-sm font-semibold text-gray-900">Stripe</span>
                    <span className="block text-sm text-gray-500">Cards and wallets supported by Stripe Checkout</span>
                  </span>
                  <CreditCardIcon className="h-6 w-6 text-indigo-600" />
                </button>

                <button
                  type="button"
                  disabled
                  className="flex w-full cursor-not-allowed items-center justify-between rounded-lg border border-gray-200 bg-gray-50 p-4 text-left opacity-60"
                >
                  <span>
                    <span className="block text-sm font-semibold text-gray-500">PayPal</span>
                    <span className="block text-sm text-gray-400">Not available yet</span>
                  </span>
                </button>
              </div>

              <div className="flex justify-end">
                <PButton variant="secondary" onClick={() => navigate(-1)}>Back</PButton>
              </div>
            </div>
          ) : null}
        </PCard>
      </div>
    </Layout>
  )
}
