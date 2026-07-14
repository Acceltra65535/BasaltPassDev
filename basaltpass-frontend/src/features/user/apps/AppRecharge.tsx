import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { getAppRechargeConfig, type AppRechargeConfig, type AppRechargeCurrency } from '@api/user/appRecharge'
import { getCurrencies, getCurrencyRates, type Currency, type CurrencyRate } from '@api/user/currency'
import { paymentAPI } from '@api/subscription/payment/payment'
import Layout from '@features/user/components/Layout'
import { PAlert, PButton, PCard, PInput, PPageHeader, PSkeleton } from '@ui'
import { ROUTES } from '@constants'
import { useConfig } from '@contexts/ConfigContext'
import { useI18n } from '@shared/i18n'
import { ArrowUpIcon, BanknotesIcon } from '@heroicons/react/24/outline'

const formatAmount = (amount: number, currency: Currency): string =>
  amount.toFixed(Math.min(currency.decimal_places, 8))

const fromSmallestUnit = (value: number, currency: Currency): number =>
  value / Math.pow(10, currency.decimal_places)

const resolveExchangeRate = (
  targetCurrency: Currency,
  paymentCurrency: Currency,
  rates: CurrencyRate[],
): number | null => {
  if (targetCurrency.code === paymentCurrency.code) return 1
  const exact = rates.find((rate) =>
    rate.base_currency_code === targetCurrency.code &&
    rate.quote_currency_code === paymentCurrency.code &&
    rate.is_active !== false &&
    Number(rate.rate) > 0
  )
  if (exact) return Number(exact.rate)
  const inverse = rates.find((rate) =>
    rate.base_currency_code === paymentCurrency.code &&
    rate.quote_currency_code === targetCurrency.code &&
    rate.is_active !== false &&
    Number(rate.rate) > 0
  )
  if (inverse) return 1 / Number(inverse.rate)
  const targetRate = Number(targetCurrency.exchange_rate_usd || 0)
  const paymentRate = Number(paymentCurrency.exchange_rate_usd || 0)
  if (targetRate > 0 && paymentRate > 0) return targetRate / paymentRate
  return null
}

const calculatePaymentAmount = (
  targetAmount: number,
  targetCurrency: Currency | null,
  paymentCurrency: Currency | null,
  rates: CurrencyRate[],
) => {
  if (!targetCurrency || !paymentCurrency || !targetAmount || targetAmount <= 0) return null
  const rate = resolveExchangeRate(targetCurrency, paymentCurrency, rates)
  if (!rate || rate <= 0) return null
  const amount = targetAmount * rate
  return {
    amount,
    smallestUnit: Math.max(1, Math.ceil(amount * Math.pow(10, paymentCurrency.decimal_places) - 1e-9)),
    rate,
  }
}

const getQuickAmounts = (currency: Currency): number[] => {
  if (currency.type === 'points') return [100000, 500000, 1000000, 5000000, 10000000, 50000000]
  if (currency.type === 'crypto') return [1, 10, 50, 100, 500, 1000]
  return [50, 100, 200, 500, 1000, 2000]
}

export default function AppRecharge() {
  const { t } = useI18n()
  const { walletRechargeWithdrawEnabled } = useConfig()
  const navigate = useNavigate()
  const params = useParams()
  const [searchParams] = useSearchParams()
  const [config, setConfig] = useState<AppRechargeConfig | null>(null)
  const [selectedCurrency, setSelectedCurrency] = useState<AppRechargeCurrency | null>(null)
  const [paymentCurrencies, setPaymentCurrencies] = useState<Currency[]>([])
  const [paymentCurrency, setPaymentCurrency] = useState<Currency | null>(null)
  const [currencyRates, setCurrencyRates] = useState<CurrencyRate[]>([])
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const walletOpsDisabled = !walletRechargeWithdrawEnabled
  const targetAmount = Number(amount)
  const estimatedPayment = useMemo(
    () => calculatePaymentAmount(targetAmount, selectedCurrency, paymentCurrency, currencyRates),
    [currencyRates, targetAmount, selectedCurrency, paymentCurrency],
  )

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const clientId = searchParams.get('client_id') || undefined
        const appId = params.id || searchParams.get('app_id') || undefined
        const [rechargeResponse, currenciesResponse, ratesResponse] = await Promise.all([
          getAppRechargeConfig({ app_id: appId, client_id: clientId, category: 'top_up' }),
          getCurrencies(),
          getCurrencyRates(),
        ])
        const nextConfig = rechargeResponse.data
        setConfig(nextConfig)
        const defaultCurrency = nextConfig.currencies.find((currency) => currency.is_default) || nextConfig.currencies[0] || null
        setSelectedCurrency(defaultCurrency)
        const fiatPaymentCurrencies = currenciesResponse.data.filter((currency) => currency.type === 'fiat' && currency.payment_enabled)
        setPaymentCurrencies(fiatPaymentCurrencies)
        setPaymentCurrency(fiatPaymentCurrencies.find((currency) => currency.code === 'USD') || fiatPaymentCurrencies[0] || null)
        setCurrencyRates(ratesResponse.data)
      } catch (e: any) {
        setError(e.response?.data?.error || t('pages.appRecharge.errors.loadFailed'))
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [params.id, searchParams, t])

  useEffect(() => {
    if (!selectedCurrency || paymentCurrencies.length === 0) return
    if (selectedCurrency.type === 'fiat') {
      setPaymentCurrency(paymentCurrencies.find((currency) => currency.code === selectedCurrency.code) || paymentCurrencies[0])
    } else if (!paymentCurrency) {
      setPaymentCurrency(paymentCurrencies.find((currency) => currency.code === 'USD') || paymentCurrencies[0])
    }
  }, [paymentCurrencies, paymentCurrency, selectedCurrency])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (walletOpsDisabled) {
      setError(t('pages.walletRecharge.errors.disabled'))
      return
    }
    if (!selectedCurrency) {
      setError(t('pages.appRecharge.errors.noCurrency'))
      return
    }
    if (!amount || Number(amount) <= 0) {
      setError(t('pages.walletRecharge.errors.invalidAmount'))
      return
    }
    if (!paymentCurrency) {
      setError(t('pages.walletRecharge.errors.selectPaymentCurrency'))
      return
    }
    if (!estimatedPayment) {
      setError(t('pages.walletRecharge.errors.exchangeRateMissing'))
      return
    }

    setSubmitting(true)
    setError('')
    try {
      const amountInSmallestUnit = Math.round(Number(amount) * Math.pow(10, selectedCurrency.decimal_places))
      const chargeCurrency = paymentCurrency.code
      const chargeAmount = estimatedPayment.smallestUnit
      const appName = config?.app.name || 'App'

      const intentResponse = await paymentAPI.createPaymentIntent({
        amount: chargeAmount,
        currency: chargeCurrency,
        description: `${appName} top up ${amount} ${selectedCurrency.code}`,
        payment_method_types: ['card'],
        confirmation_method: 'automatic',
        capture_method: 'automatic',
        metadata: {
          source: 'wallet_recharge',
          checkout_kind: 'top_up',
          app_id: config?.app.id ? String(config.app.id) : '',
          app_name: appName,
          wallet_currency: selectedCurrency.code,
          target_wallet_currency: selectedCurrency.code,
          target_wallet_amount: String(amountInSmallestUnit),
          charge_currency: chargeCurrency,
          charge_amount: String(chargeAmount),
        },
      })

      const currentPath = `${window.location.pathname}${window.location.search}`
      const sessionResponse = await paymentAPI.createPaymentSession({
        payment_intent_id: intentResponse.payment_intent.ID,
        success_url: `${window.location.origin}${ROUTES.user.wallet}?topup=success`,
        cancel_url: `${window.location.origin}${currentPath}`,
        user_email: '',
      })

      navigate(`/checkout?kind=top_up&session=${encodeURIComponent(sessionResponse.session.StripeSessionID)}`)
    } catch (e: any) {
      setError(e.response?.data?.error || t('pages.walletRecharge.errors.rechargeFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <Layout>
        <div className="py-6">
          <PSkeleton.Content cards={2} />
        </div>
      </Layout>
    )
  }

  return (
    <Layout>
      <div className="space-y-6">
        <PPageHeader
          title={t('pages.appRecharge.header.title', { app: config?.app.name || t('pages.appRecharge.header.appFallback') })}
          description={t('pages.appRecharge.header.description')}
          backTo={ROUTES.user.wallet}
          backLabel={t('pages.walletRecharge.header.back')}
        />

        {walletOpsDisabled && <PAlert variant="warning" message={t('pages.walletRecharge.notice.disabled')} />}
        {error && <PAlert variant="error" title={t('pages.walletRecharge.errors.title')} message={error} />}

        {config && config.currencies.length === 0 ? (
          <PCard>
            <div className="text-sm text-gray-600">{t('pages.appRecharge.empty')}</div>
          </PCard>
        ) : (
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <PCard padding="none" className={walletOpsDisabled ? 'opacity-60' : ''}>
              <div className="px-4 py-5 sm:p-6">
                <div className="mb-6 flex items-center">
                  <ArrowUpIcon className="mr-2 h-6 w-6 text-indigo-600" />
                  <h3 className="text-lg font-medium text-gray-900">{t('pages.appRecharge.form.title')}</h3>
                </div>

                <form onSubmit={submit} className={`space-y-6 ${walletOpsDisabled ? 'pointer-events-none' : ''}`}>
                  <div>
                    <label className="mb-2 block text-sm font-medium text-gray-700">
                      {t('pages.appRecharge.form.currencyLabel')}
                    </label>
                    <select
                      value={selectedCurrency?.code || ''}
                      onChange={(event) => {
                        setSelectedCurrency(config?.currencies.find((currency) => currency.code === event.target.value) || null)
                        setError('')
                      }}
                      className="min-h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    >
                      {(config?.currencies || []).map((currency) => (
                        <option key={currency.code} value={currency.code}>
                          {currency.name_cn || currency.name} ({currency.code})
                        </option>
                      ))}
                    </select>
                  </div>

                  {selectedCurrency && selectedCurrency.type !== 'fiat' && (
                    <div>
                      <label className="mb-2 block text-sm font-medium text-gray-700">
                        {t('pages.walletRecharge.form.paymentCurrencyLabel')}
                      </label>
                      <select
                        value={paymentCurrency?.code || ''}
                        onChange={(event) => {
                          setPaymentCurrency(paymentCurrencies.find((currency) => currency.code === event.target.value) || null)
                          setError('')
                        }}
                        className="min-h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      >
                        <option value="">{t('pages.walletRecharge.form.paymentCurrencyPlaceholder')}</option>
                        {paymentCurrencies.map((currency) => (
                          <option key={currency.code} value={currency.code}>
                            {currency.name_cn || currency.name} ({currency.code})
                          </option>
                        ))}
                      </select>
                    </div>
                  )}

                  <PInput
                    id="amount"
                    type="number"
                    label={t('pages.walletRecharge.form.amountLabel', { currency: selectedCurrency?.code || '' })}
                    value={amount}
                    onChange={(event) => {
                      setAmount(event.target.value)
                      setError('')
                    }}
                    placeholder={t('pages.walletRecharge.form.amountPlaceholder')}
                    min="0.01"
                    step={selectedCurrency ? `0.${'0'.repeat(Math.max(0, selectedCurrency.decimal_places - 1))}1` : '0.01'}
                  />

                  {selectedCurrency && (
                    <div>
                      <label className="mb-2 block text-sm font-medium text-gray-700">
                        {t('pages.walletRecharge.form.quickAmountLabel')}
                      </label>
                      <div className="grid grid-cols-3 gap-2">
                        {getQuickAmounts(selectedCurrency).map((value) => (
                          <PButton
                            key={value}
                            type="button"
                            onClick={() => {
                              setAmount(value.toString())
                              setError('')
                            }}
                            variant={amount === value.toString() ? 'primary' : 'secondary'}
                            size="sm"
                          >
                            {selectedCurrency.symbol}{formatAmount(value, selectedCurrency)}
                          </PButton>
                        ))}
                      </div>
                    </div>
                  )}

                  {selectedCurrency && paymentCurrency && estimatedPayment && (
                    <div className="rounded-lg border border-indigo-100 bg-indigo-50 px-4 py-3 text-sm text-indigo-900">
                      <div className="font-medium">{t('pages.walletRecharge.form.paymentEstimateTitle')}</div>
                      <div className="mt-1">
                        {t('pages.walletRecharge.form.paymentEstimate', {
                          target: `${formatAmount(targetAmount, selectedCurrency)} ${selectedCurrency.code}`,
                          payment: `${paymentCurrency.symbol}${fromSmallestUnit(estimatedPayment.smallestUnit, paymentCurrency).toFixed(paymentCurrency.decimal_places)} ${paymentCurrency.code}`,
                        })}
                      </div>
                    </div>
                  )}

                  <PButton
                    type="submit"
                    disabled={walletOpsDisabled || !amount || Number(amount) <= 0 || !selectedCurrency || !paymentCurrency || !estimatedPayment}
                    loading={submitting}
                    fullWidth
                  >
                    {t('pages.walletRecharge.form.continueToCheckout')}
                  </PButton>
                </form>
              </div>
            </PCard>

            <PCard padding="none">
              <div className="px-4 py-5 sm:p-6">
                <div className="mb-4 flex items-center">
                  <BanknotesIcon className="mr-2 h-6 w-6 text-indigo-600" />
                  <h3 className="text-lg font-medium text-gray-900">{t('pages.appRecharge.guide.title')}</h3>
                </div>
                <div className="space-y-3 text-sm text-gray-600">
                  <p>{t('pages.appRecharge.guide.body', { app: config?.app.name || t('pages.appRecharge.header.appFallback') })}</p>
                  <p>{t('pages.appRecharge.guide.currencyCount', { count: config?.currencies.length || 0 })}</p>
                </div>
              </div>
            </PCard>
          </div>
        )}
      </div>
    </Layout>
  )
}
