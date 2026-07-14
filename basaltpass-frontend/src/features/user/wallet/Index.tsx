import { useEffect, useMemo, useState } from 'react'
import { getAccounts, history as getHistory, type WalletAccount, type WalletTransaction } from '@api/user/wallet'
import { getCurrencyRates, type Currency, type CurrencyRate } from '@api/user/currency'
import { Link } from 'react-router-dom'
import Layout from '@features/user/components/Layout'
import { ROUTES } from '@constants'
import { useConfig } from '@contexts/ConfigContext'
import { useI18n } from '@shared/i18n'
import { PSkeleton, PPageHeader, PBadge, PCard } from '@ui'
import {
  WalletIcon,
  ArrowUpIcon,
  ArrowDownIcon,
  ClockIcon,
  GiftIcon,
  CurrencyDollarIcon,
  ChartBarIcon,
} from '@heroicons/react/24/outline'

const USD_CODE = 'USD'

const getDirection = (type: string, amount: number): 'in' | 'out' => {
  const normalized = (type || '').toLowerCase()
  const inKeywords = ['recharge', 'deposit', 'increase', 'refund', 'income']
  const outKeywords = ['withdraw', 'decrease', 'debit', 'consume', 'payment', 'expense']
  if (inKeywords.some((keyword) => normalized.includes(keyword))) return 'in'
  if (outKeywords.some((keyword) => normalized.includes(keyword))) return 'out'
  return amount >= 0 ? 'in' : 'out'
}

const resolveExchangeRate = (from: Currency | null | undefined, toCode: string, rates: CurrencyRate[]) => {
  if (!from) return null
  if (from.code === toCode) return 1
  const exact = rates.find((rate) =>
    rate.base_currency_code === from.code &&
    rate.quote_currency_code === toCode &&
    rate.is_active !== false &&
    Number(rate.rate) > 0
  )
  if (exact) return Number(exact.rate)
  const inverse = rates.find((rate) =>
    rate.base_currency_code === toCode &&
    rate.quote_currency_code === from.code &&
    rate.is_active !== false &&
    Number(rate.rate) > 0
  )
  if (inverse) return 1 / Number(inverse.rate)
  if (toCode === USD_CODE && Number(from.exchange_rate_usd || 0) > 0) return Number(from.exchange_rate_usd)
  return null
}

const amountToUnits = (amountMinor: number, currency?: Currency | null) => {
  if (!currency) return amountMinor
  return amountMinor / Math.pow(10, Math.max(0, currency.decimal_places || 0))
}

const formatCurrency = (amountMinor: number, currency?: Currency | null) => {
  if (!currency) return '--'
  const places = Math.min(Math.max(0, currency.decimal_places || 0), 8)
  let text = amountToUnits(amountMinor, currency).toFixed(places)
  if (places > 2) text = text.replace(/0+$/, '').replace(/\.$/, '')
  return `${currency.symbol || ''}${text} ${currency.code}`
}

const formatUSD = (amount: number) =>
  new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(amount)

export default function WalletIndex() {
  const { walletRechargeWithdrawEnabled, walletWithdrawEnabled } = useConfig()
  const { t, locale } = useI18n()
  const [accounts, setAccounts] = useState<WalletAccount[]>([])
  const [rates, setRates] = useState<CurrencyRate[]>([])
  const [selectedCode, setSelectedCode] = useState('')
  const [txs, setTxs] = useState<WalletTransaction[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)

  const selectedAccount = useMemo(
    () => accounts.find((account) => account.Currency?.code === selectedCode) || accounts[0] || null,
    [accounts, selectedCode],
  )

  const accountRows = useMemo(() => {
    return accounts.map((account) => {
      const currency = account.Currency
      const units = amountToUnits(account.Balance, currency)
      const rate = resolveExchangeRate(currency, USD_CODE, rates)
      return {
        account,
        currency,
        usdValue: rate === null ? null : units * rate,
      }
    })
  }, [accounts, rates])

  const totalUSD = accountRows.reduce((sum, row) => sum + (row.usdValue || 0), 0)

  const monthly = useMemo(() => {
    if (!selectedAccount?.Currency) return { income: 0, expense: 0 }
    const now = new Date()
    let income = 0
    let expense = 0
    for (const tx of txs) {
      const d = new Date(tx.CreatedAt)
      if (d.getFullYear() !== now.getFullYear() || d.getMonth() !== now.getMonth()) continue
      if (getDirection(tx.Type, tx.Amount) === 'in') income += Math.abs(tx.Amount)
      else expense += Math.abs(tx.Amount)
    }
    return { income, expense }
  }, [selectedAccount, txs])

  useEffect(() => {
    const load = async () => {
      setIsLoading(true)
      try {
        const [accountsRes, ratesRes] = await Promise.all([getAccounts(), getCurrencyRates()])
        const list = accountsRes.data || []
        setAccounts(list)
        setRates(ratesRes.data || [])
        const firstNonZero = list.find((account) => account.Balance !== 0) || list[0]
        setSelectedCode(firstNonZero?.Currency?.code || '')
      } catch (error) {
        console.error('Failed to load wallet accounts:', error)
        setAccounts([])
        setRates([])
      } finally {
        setIsLoading(false)
      }
    }
    load()
  }, [])

  useEffect(() => {
    if (!selectedAccount?.Currency?.code) {
      setTxs([])
      return
    }
    setIsDetailLoading(true)
    getHistory(selectedAccount.Currency.code, 200)
      .then((res) => setTxs(res.data || []))
      .catch((error) => {
        console.error('Failed to load wallet transactions:', error)
        setTxs([])
      })
      .finally(() => setIsDetailLoading(false))
  }, [selectedAccount?.Currency?.code])

  const getTypeLabel = (type: string) => {
    const normalized = (type || '').toLowerCase()
    if (normalized === 'recharge') return t('pages.wallet.transactionTypes.recharge')
    if (normalized === 'withdraw') return t('pages.wallet.transactionTypes.withdraw')
    if (normalized === 'admin_deposit') return t('pages.wallet.transactionTypes.adminDeposit')
    if (normalized === 's2s_wallet_increase') return t('pages.wallet.transactionTypes.apiIncrease')
    if (normalized === 's2s_wallet_decrease') return t('pages.wallet.transactionTypes.apiDecrease')
    return type || t('pages.wallet.transactionTypes.default')
  }

  if (isLoading) {
    return (
      <Layout>
        <div className="py-6">
          <PSkeleton.Content cards={3} />
        </div>
      </Layout>
    )
  }

  const selectedCurrency = selectedAccount?.Currency || null

  return (
    <Layout>
      <div className="space-y-6">
        <PPageHeader title={t('pages.wallet.title')} description={t('pages.wallet.description')} />

        <div className="rounded-lg bg-indigo-600 shadow-lg">
          <div className="px-6 py-8">
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-indigo-100">{t('pages.wallet.overview.totalUsd')}</p>
                <p className="text-4xl font-bold text-white">{formatUSD(totalUSD)}</p>
                <p className="mt-1 text-sm text-indigo-100">
                  {t('pages.wallet.overview.accountCount', { count: accounts.length })}
                </p>
                <p className="mt-1 text-xs text-indigo-100">
                  {t('pages.wallet.overview.lastUpdated', { time: new Date().toLocaleString(locale) })}
                </p>
              </div>
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-white bg-opacity-20">
                <WalletIcon className="h-8 w-8 text-white" />
              </div>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-5 sm:grid-cols-3">
          <PCard className="relative overflow-hidden px-4 py-5 sm:px-6">
            <div className="absolute rounded-lg bg-indigo-100 p-3">
              <CurrencyDollarIcon className="h-6 w-6 text-indigo-600" />
            </div>
            <p className="ml-16 truncate text-sm font-medium text-gray-500">{t('pages.wallet.stats.totalUsd')}</p>
            <p className="ml-16 text-2xl font-semibold text-indigo-600">{formatUSD(totalUSD)}</p>
          </PCard>
          <PCard className="relative overflow-hidden px-4 py-5 sm:px-6">
            <div className="absolute rounded-lg bg-green-100 p-3">
              <ChartBarIcon className="h-6 w-6 text-green-600" />
            </div>
            <p className="ml-16 truncate text-sm font-medium text-gray-500">{t('pages.wallet.stats.monthlyIncome')}</p>
            <p className="ml-16 text-2xl font-semibold text-green-600">
              +{selectedCurrency ? formatCurrency(monthly.income, selectedCurrency) : '--'}
            </p>
          </PCard>
          <PCard className="relative overflow-hidden px-4 py-5 sm:px-6">
            <div className="absolute rounded-lg bg-red-100 p-3">
              <ChartBarIcon className="h-6 w-6 text-red-600" />
            </div>
            <p className="ml-16 truncate text-sm font-medium text-gray-500">{t('pages.wallet.stats.monthlyExpense')}</p>
            <p className="ml-16 text-2xl font-semibold text-red-600">
              -{selectedCurrency ? formatCurrency(monthly.expense, selectedCurrency) : '--'}
            </p>
          </PCard>
        </div>

        <PCard padding="none">
          <div className="px-4 py-5 sm:p-6">
            <h3 className="mb-4 text-lg font-medium leading-6 text-gray-900">{t('pages.wallet.accounts.title')}</h3>
            {accountRows.length === 0 ? (
              <div className="rounded-lg border border-dashed border-gray-300 px-6 py-8 text-center text-sm text-gray-500">
                {t('pages.wallet.accounts.empty')}
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                {accountRows.map(({ account, currency, usdValue }) => {
                  const active = selectedAccount?.ID === account.ID
                  return (
                    <button
                      key={account.ID}
                      type="button"
                      onClick={() => setSelectedCode(currency?.code || '')}
                      className={`rounded-lg border px-4 py-3 text-left transition-colors ${
                        active ? 'border-indigo-500 bg-indigo-50 ring-2 ring-indigo-100' : 'border-gray-200 bg-white hover:border-indigo-300'
                      }`}
                    >
                      <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-semibold text-gray-900">
                            {currency?.name_cn || currency?.name || t('pages.wallet.accounts.unknown')}
                          </div>
                          <div className="text-xs text-gray-500">{currency?.code || '--'}</div>
                        </div>
                        <PBadge variant={active ? 'info' : 'default'}>{currency?.type || 'wallet'}</PBadge>
                      </div>
                      <div className="mt-3 text-xl font-semibold text-gray-900">{formatCurrency(account.Balance, currency)}</div>
                      <div className="mt-1 text-sm text-gray-500">
                        {usdValue === null ? t('pages.wallet.accounts.noRate') : formatUSD(usdValue)}
                      </div>
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        </PCard>

        <PCard padding="none">
          <div className="px-4 py-5 sm:p-6">
            <h3 className="mb-4 text-lg font-medium leading-6 text-gray-900">{t('pages.wallet.quickActions.title')}</h3>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
              {walletRechargeWithdrawEnabled ? (
                <Link to={ROUTES.user.walletRecharge} className="relative flex items-center gap-3 rounded-lg border border-gray-300 bg-white px-5 py-4 shadow-sm transition-colors hover:border-indigo-300 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2">
                  <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-indigo-100">
                    <ArrowUpIcon className="h-6 w-6 text-indigo-600" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <span className="absolute inset-0" aria-hidden="true" />
                    <p className="text-sm font-medium text-gray-900">{t('pages.wallet.quickActions.recharge')}</p>
                    <p className="text-sm text-gray-500">{t('pages.wallet.quickActions.rechargeDesc')}</p>
                  </div>
                </Link>
              ) : null}
              {walletWithdrawEnabled ? (
                <Link to={ROUTES.user.walletWithdraw} className="relative flex items-center gap-3 rounded-lg border border-gray-300 bg-white px-5 py-4 shadow-sm transition-colors hover:border-indigo-300 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2">
                  <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-indigo-100">
                    <ArrowDownIcon className="h-6 w-6 text-indigo-600" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <span className="absolute inset-0" aria-hidden="true" />
                    <p className="text-sm font-medium text-gray-900">{t('pages.wallet.quickActions.withdraw')}</p>
                    <p className="text-sm text-gray-500">{t('pages.wallet.quickActions.withdrawDesc')}</p>
                  </div>
                </Link>
              ) : null}
              <Link to={ROUTES.user.walletHistory} className="relative flex items-center gap-3 rounded-lg border border-gray-300 bg-white px-5 py-4 shadow-sm transition-colors hover:border-indigo-300 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2">
                <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-indigo-100">
                  <ClockIcon className="h-6 w-6 text-indigo-600" />
                </div>
                <div className="min-w-0 flex-1">
                  <span className="absolute inset-0" aria-hidden="true" />
                  <p className="text-sm font-medium text-gray-900">{t('pages.wallet.quickActions.history')}</p>
                  <p className="text-sm text-gray-500">{t('pages.wallet.quickActions.historyDesc')}</p>
                </div>
              </Link>
              <Link to={ROUTES.user.walletGiftCardRedeem} className="relative flex items-center gap-3 rounded-lg border border-gray-300 bg-white px-5 py-4 shadow-sm transition-colors hover:border-indigo-300 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2">
                <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-indigo-100">
                  <GiftIcon className="h-6 w-6 text-indigo-600" />
                </div>
                <div className="min-w-0 flex-1">
                  <span className="absolute inset-0" aria-hidden="true" />
                  <p className="text-sm font-medium text-gray-900">{t('pages.wallet.quickActions.redeemGiftCard')}</p>
                  <p className="text-sm text-gray-500">{t('pages.wallet.quickActions.redeemGiftCardDesc')}</p>
                </div>
              </Link>
            </div>
          </div>
        </PCard>

        <PCard padding="none">
          <div className="px-4 py-5 sm:p-6">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-lg font-medium leading-6 text-gray-900">
                {selectedCurrency
                  ? t('pages.wallet.recentTransactions.titleWithCode', { code: selectedCurrency.code })
                  : t('pages.wallet.recentTransactions.title')}
              </h3>
              <Link to={ROUTES.user.walletHistory} className="text-sm font-medium text-indigo-600 hover:text-indigo-700">
                {t('pages.wallet.recentTransactions.viewAll')}
              </Link>
            </div>
            {isDetailLoading ? (
              <PSkeleton.List items={3} />
            ) : txs.length === 0 ? (
              <div className="py-8 text-center text-sm text-gray-500">{t('pages.wallet.recentTransactions.empty')}</div>
            ) : (
              <div className="flow-root">
                <ul className="-my-5 divide-y divide-gray-200">
                  {txs.slice(0, 5).map((tx) => {
                    const direction = getDirection(tx.Type, tx.Amount)
                    const isIncoming = direction === 'in'
                    const statusLower = (tx.Status || '').toLowerCase()
                    const statusVariant = statusLower === 'success' || statusLower === 'completed'
                      ? 'success' as const
                      : statusLower === 'pending'
                        ? 'warning' as const
                        : 'default' as const
                    const txCurrency = tx.Wallet?.Currency || selectedCurrency
                    return (
                      <li key={tx.ID} className="py-4">
                        <div className="flex items-center space-x-4">
                          <div className="flex-shrink-0">
                            <div className={`flex h-8 w-8 items-center justify-center rounded-full ${isIncoming ? 'bg-green-100' : 'bg-red-100'}`}>
                              {isIncoming ? <ArrowUpIcon className="h-4 w-4 text-green-600" /> : <ArrowDownIcon className="h-4 w-4 text-red-600" />}
                            </div>
                          </div>
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm font-medium text-gray-900">{getTypeLabel(tx.Type)} #{tx.ID}</p>
                            <p className="text-sm text-gray-500">{new Date(tx.CreatedAt).toLocaleString(locale)}</p>
                          </div>
                          <div className="flex-shrink-0 text-right">
                            <p className={`text-sm font-medium ${isIncoming ? 'text-green-600' : 'text-red-600'}`}>
                              {isIncoming ? '+' : '-'}{formatCurrency(Math.abs(tx.Amount), txCurrency)}
                            </p>
                            <PBadge variant={statusVariant}>
                              {statusLower === 'success' || statusLower === 'completed'
                                ? t('pages.wallet.recentTransactions.status.completed')
                                : statusLower === 'pending'
                                  ? t('pages.wallet.recentTransactions.status.processing')
                                  : (tx.Status || t('pages.wallet.recentTransactions.status.unknown'))}
                            </PBadge>
                          </div>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              </div>
            )}
          </div>
        </PCard>
      </div>
    </Layout>
  )
}
