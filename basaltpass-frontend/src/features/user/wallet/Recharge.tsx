import { useEffect, useMemo, useState } from 'react'
import { Currency, CurrencyRate, getCurrencies, getCurrencyRates } from '@api/user/currency'
import { useNavigate } from 'react-router-dom'
import { paymentAPI } from '@api/subscription/payment/payment'
import Layout from '@features/user/components/Layout'
import CurrencySelector from '@features/user/components/CurrencySelector'
import { PInput, PButton, PPageHeader, PCard, PAlert } from '@ui'
import { ROUTES } from '@constants'
import { useConfig } from '@contexts/ConfigContext'
import { useI18n } from '@shared/i18n'
import { 
  ArrowUpIcon,
  CreditCardIcon,
  BanknotesIcon,
  GiftIcon,
} from '@heroicons/react/24/outline'

const quickAmounts = [50, 100, 200, 500, 1000, 2000]

// 
const getQuickAmounts = (currency: Currency): number[] => {
  switch (currency.type) {
    case 'crypto':
      if (currency.code === 'BTC') {
        return [0.001, 0.005, 0.01, 0.05, 0.1, 0.5]
      } else if (currency.code === 'ETH') {
        return [0.01, 0.05, 0.1, 0.5, 1, 5]
      }
      return [1, 10, 50, 100, 500, 1000]
    case 'points':
      return [100, 500, 1000, 5000, 10000, 50000]
    default: // fiat
      return quickAmounts
  }
}

// 
const formatAmount = (amount: number, currency: Currency): string => {
  return amount.toFixed(Math.min(currency.decimal_places, 8))
}

const getAmountInputConstraints = (currency: Currency | null) => {
  if (!currency) return { min: '0.01', step: '0.01' }
  const decimals = Math.max(0, currency.decimal_places || 0)
  if (decimals === 0) return { min: '1', step: '1' }
  return {
    min: (1 / Math.pow(10, decimals)).toFixed(decimals),
    step: (1 / Math.pow(10, decimals)).toFixed(decimals),
  }
}

const fromSmallestUnit = (value: number, currency: Currency): number =>
  value / Math.pow(10, currency.decimal_places)

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

export default function Recharge() {
  const { t } = useI18n()
  const { walletRechargeWithdrawEnabled } = useConfig()
  const walletOpsDisabled = !walletRechargeWithdrawEnabled
  const navigate = useNavigate()
  const [amount, setAmount] = useState('')
  const [selectedCurrency, setSelectedCurrency] = useState<Currency | null>(null)
  const [paymentCurrencies, setPaymentCurrencies] = useState<Currency[]>([])
  const [currencyRates, setCurrencyRates] = useState<CurrencyRate[]>([])
  const [paymentCurrency, setPaymentCurrency] = useState<Currency | null>(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const targetAmount = Number(amount)
  const requiresPaymentCurrency = !!selectedCurrency && selectedCurrency.type !== 'fiat'
  const estimatedPayment = useMemo(
    () => calculatePaymentAmount(targetAmount, selectedCurrency, paymentCurrency, currencyRates),
    [currencyRates, targetAmount, selectedCurrency, paymentCurrency],
  )
  const amountInputConstraints = getAmountInputConstraints(selectedCurrency)

  useEffect(() => {
    Promise.all([getCurrencies(), getCurrencyRates()])
      .then(([currenciesResponse, ratesResponse]) => {
        const currencies = currenciesResponse.data.filter((currency) => currency.type === 'fiat' && currency.payment_enabled)
        setPaymentCurrencies(currencies)
        setCurrencyRates(ratesResponse.data)
      })
      .catch(() => {
        setPaymentCurrencies([])
        setCurrencyRates([])
      })
  }, [])

  useEffect(() => {
    if (!selectedCurrency) return
    const matchingPaymentCurrency = paymentCurrencies.find((currency) => currency.code === selectedCurrency.code)
    if (selectedCurrency.type === 'fiat' && matchingPaymentCurrency) {
      setPaymentCurrency(matchingPaymentCurrency)
      return
    }
    if (!paymentCurrency && paymentCurrencies.length > 0) {
      setPaymentCurrency(paymentCurrencies.find((currency) => currency.code === 'USD') || paymentCurrencies[0])
    }
  }, [paymentCurrencies, paymentCurrency, selectedCurrency])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (walletOpsDisabled) {
      setError(t('pages.walletRecharge.errors.disabled'))
      return
    }
    
    if (!amount || parseFloat(amount) <= 0) {
      setError(t('pages.walletRecharge.errors.invalidAmount'))
      return
    }

    if (!selectedCurrency) {
      setError(t('pages.walletRecharge.errors.selectCurrency'))
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

    setIsLoading(true)
    setError('')
    
    try {
      const decimals = selectedCurrency.decimal_places
      const amountInSmallestUnit = Math.round(Number(amount) * Math.pow(10, decimals))
      const chargeCurrency = paymentCurrency.code
      const chargeAmount = estimatedPayment.smallestUnit

      const intentResponse = await paymentAPI.createPaymentIntent({
        amount: chargeAmount,
        currency: chargeCurrency,
        description: `Top up ${amount} ${selectedCurrency.code}`,
        payment_method_types: ['card'],
        confirmation_method: 'automatic',
        capture_method: 'automatic',
        metadata: {
          source: 'wallet_recharge',
          checkout_kind: 'top_up',
          wallet_currency: selectedCurrency.code,
          target_wallet_currency: selectedCurrency.code,
          target_wallet_amount: String(amountInSmallestUnit),
          charge_currency: chargeCurrency,
          charge_amount: String(chargeAmount),
        },
      })

      const sessionResponse = await paymentAPI.createPaymentSession({
        payment_intent_id: intentResponse.payment_intent.ID,
        success_url: `${window.location.origin}${ROUTES.user.wallet}?topup=success&session_id={CHECKOUT_SESSION_ID}`,
        cancel_url: `${window.location.origin}${ROUTES.user.walletRecharge}?topup=canceled`,
        user_email: '',
      })

      navigate(`/checkout?kind=top_up&session=${encodeURIComponent(sessionResponse.session.StripeSessionID)}`)
    } catch (e: any) {
      setError(e.response?.data?.error || t('pages.walletRecharge.errors.rechargeFailed'))
    } finally {
      setIsLoading(false)
    }
  }

  const handleQuickAmount = (value: number) => {
    setAmount(value.toString())
    setError('')
  }

  const handleAmountChange = (value: string) => {
    setAmount(value)
    setError('')
  }

  return (
    <Layout>
      <div className="space-y-6">
        {/*  */}
        <PPageHeader
          title={t('pages.walletRecharge.header.title')}
          description={t('pages.walletRecharge.header.description')}
          backTo={ROUTES.user.wallet}
          backLabel={t('pages.walletRecharge.header.back')}
        />

        {walletOpsDisabled && (
          <PAlert variant="warning" message={t('pages.walletRecharge.notice.disabled')} />
        )}

        <div className="flex justify-end">
          <PButton
            type="button"
            variant="secondary"
            onClick={() => navigate(ROUTES.user.walletGiftCardRedeem)}
          >
            <span className="inline-flex items-center gap-2">
              <GiftIcon className="h-5 w-5" />
              {t('pages.walletRecharge.giftCard.title')}
            </span>
          </PButton>
        </div>

        {/*  */}
        {error && (
          <PAlert variant="error" title={t('pages.walletRecharge.errors.title')} message={error} />
        )}

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          {/*  */}
          <PCard padding="none" className={walletOpsDisabled ? 'opacity-60' : ''}>
            <div className="px-4 py-5 sm:p-6">
              <div className="flex items-center mb-6">
                <ArrowUpIcon className="h-6 w-6 text-indigo-600 mr-2" />
                <h3 className="text-lg font-medium text-gray-900">{t('pages.walletRecharge.form.title')}</h3>
              </div>
              
              <form onSubmit={submit} className={`space-y-6 ${walletOpsDisabled ? 'pointer-events-none' : ''}`}>
                {/*  */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    {t('pages.walletRecharge.form.currencyLabel')}
                  </label>
                  <CurrencySelector
                    value={selectedCurrency?.code || ''}
                    onChange={(currency) => {
                      setSelectedCurrency(currency)
                      setError('')
                    }}
                    className="w-full"
                  />
                </div>

                {requiresPaymentCurrency && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      {t('pages.walletRecharge.form.paymentCurrencyLabel')}
                    </label>
                    <select
                      value={paymentCurrency?.code || ''}
                      onChange={(e) => {
                        const next = paymentCurrencies.find((currency) => currency.code === e.target.value) || null
                        setPaymentCurrency(next)
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

                {/*  */}
                <div>
                  <PInput
                    id="amount"
                    type="number"
                    label={t('pages.walletRecharge.form.amountLabel', { currency: selectedCurrency?.code || '' })}
                    value={amount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    placeholder={t('pages.walletRecharge.form.amountPlaceholder')}
                    min={amountInputConstraints.min}
                    step={amountInputConstraints.step}
                  />
                </div>

                {/*  */}
                {selectedCurrency && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      {t('pages.walletRecharge.form.quickAmountLabel')}
                    </label>
                    <div className="grid grid-cols-3 gap-2">
                      {getQuickAmounts(selectedCurrency).map((value) => (
                        <PButton
                          key={value}
                          type="button"
                          onClick={() => handleQuickAmount(value)}
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
                    {requiresPaymentCurrency && (
                      <div className="mt-1 text-xs text-indigo-700">
                        {t('pages.walletRecharge.form.exchangeRateHint')}
                      </div>
                    )}
                  </div>
                )}

                {/*  */}
                <PButton
                  type="submit"
                  disabled={walletOpsDisabled || !amount || parseFloat(amount) <= 0 || !selectedCurrency || !paymentCurrency || !estimatedPayment}
                  loading={isLoading}
                  fullWidth
                >
                  {t('pages.walletRecharge.form.continueToCheckout')}
                </PButton>
              </form>
            </div>
          </PCard>

          {/*  */}
          <div className="space-y-6">
            {/*  */}
            <PCard padding="none">
              <div className="px-4 py-5 sm:p-6">
                <div className="flex items-center mb-4">
                  <BanknotesIcon className="h-6 w-6 text-indigo-600 mr-2" />
                  <h3 className="text-lg font-medium text-gray-900">{t('pages.walletRecharge.guide.title')}</h3>
                </div>
                <div className="space-y-3 text-sm text-gray-600">
                  <div className="flex items-start">
                    <div className="h-2 w-2 bg-indigo-400 rounded-full mt-2 mr-3 flex-shrink-0"></div>
                    <p>{t('pages.walletRecharge.guide.items.realtime')}</p>
                  </div>
                  <div className="flex items-start">
                    <div className="h-2 w-2 bg-indigo-400 rounded-full mt-2 mr-3 flex-shrink-0"></div>
                    <p>{t('pages.walletRecharge.guide.items.methods')}</p>
                  </div>
                  <div className="flex items-start">
                    <div className="h-2 w-2 bg-indigo-400 rounded-full mt-2 mr-3 flex-shrink-0"></div>
                    <p>{t('pages.walletRecharge.guide.items.limit')}</p>
                  </div>
                  <div className="flex items-start">
                    <div className="h-2 w-2 bg-indigo-400 rounded-full mt-2 mr-3 flex-shrink-0"></div>
                    <p>{t('pages.walletRecharge.guide.items.fee')}</p>
                  </div>
                </div>
              </div>
            </PCard>

            {/*  */}
            <PAlert variant="info" title={t('pages.walletRecharge.security.title')}>
              <ul className="list-disc list-inside space-y-1">
                <li>{t('pages.walletRecharge.security.items.network')}</li>
                <li>{t('pages.walletRecharge.security.items.password')}</li>
                <li>{t('pages.walletRecharge.security.items.support')}</li>
              </ul>
            </PAlert>

          </div>
        </div>
      </div>
    </Layout>
  )
} 
