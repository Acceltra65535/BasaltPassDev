import { useState } from 'react'
import { Currency } from '@api/user/currency'
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

export default function Recharge() {
  const { t } = useI18n()
  const { walletRechargeWithdrawEnabled } = useConfig()
  const walletOpsDisabled = !walletRechargeWithdrawEnabled
  const navigate = useNavigate()
  const [amount, setAmount] = useState('')
  const [selectedCurrency, setSelectedCurrency] = useState<Currency | null>(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)

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

    setIsLoading(true)
    setError('')
    
    try {
      const decimals = selectedCurrency.decimal_places
      const amountInSmallestUnit = Math.round(Number(amount) * Math.pow(10, decimals))
      const chargeCurrency = selectedCurrency.code.length === 3 ? selectedCurrency.code : 'USD'
      const chargeAmount = selectedCurrency.code.length === 3
        ? amountInSmallestUnit
        : Math.round(Number(amount) * 100)

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
        success_url: `${window.location.origin}${ROUTES.user.wallet}?topup=success`,
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
                    onChange={setSelectedCurrency}
                    className="w-full"
                  />
                </div>

                {/*  */}
                <div>
                  <PInput
                    id="amount"
                    type="number"
                    label={t('pages.walletRecharge.form.amountLabel', { currency: selectedCurrency?.code || '' })}
                    value={amount}
                    onChange={(e) => handleAmountChange(e.target.value)}
                    placeholder={t('pages.walletRecharge.form.amountPlaceholder')}
                    min="0.01"
                    step={selectedCurrency ? `0.${'0'.repeat(Math.max(0, selectedCurrency.decimal_places - 1))}1` : "0.01"}
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

                {/*  */}
                <PButton
                  type="submit"
                  disabled={walletOpsDisabled || !amount || parseFloat(amount) <= 0 || !selectedCurrency}
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
