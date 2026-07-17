import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BanknotesIcon,
  WalletIcon,
} from '@heroicons/react/24/outline'
import Layout from '@features/user/components/Layout'
import {
  teamApi,
  type TeamResponse,
  type TeamWalletAccount,
  type TeamWalletTransaction,
} from '@api/user/team'
import { PAlert, PBadge, PCard, PPageHeader, PSkeleton } from '@ui'
import { useI18n } from '@shared/i18n'

const formatAmount = (amount: number, account: TeamWalletAccount | null) => {
  const currency = account?.currency
  if (!currency) return String(amount)
  const places = Math.max(0, currency.decimal_places || 0)
  const value = amount / Math.pow(10, places)
  const formatted = new Intl.NumberFormat(undefined, {
    minimumFractionDigits: Math.min(places, 2),
    maximumFractionDigits: Math.min(places, 8),
  }).format(value)
  return `${currency.symbol || ''}${formatted} ${currency.code}`
}

const isIncomingTransaction = (tx: TeamWalletTransaction) => {
  const type = tx.type.toLowerCase()
  if (['withdraw', 'decrease', 'debit', 'usage', 'charge'].some((part) => type.includes(part))) return false
  if (['recharge', 'increase', 'deposit', 'refund', 'credit'].some((part) => type.includes(part))) return true
  return tx.amount >= 0
}

export default function TeamWallet() {
  const { id } = useParams<{ id: string }>()
  const teamID = Number(id)
  const { t, locale } = useI18n()
  const [team, setTeam] = useState<TeamResponse | null>(null)
  const [accounts, setAccounts] = useState<TeamWalletAccount[]>([])
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [transactions, setTransactions] = useState<TeamWalletTransaction[]>([])
  const [loading, setLoading] = useState(true)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [error, setError] = useState('')

  const selectedAccount = useMemo(
    () => accounts.find((account) => account.id === selectedID) || accounts[0] || null,
    [accounts, selectedID],
  )

  useEffect(() => {
    if (!Number.isInteger(teamID) || teamID <= 0) {
      setError(t('pages.teamWallet.errors.invalidTeam'))
      setLoading(false)
      return
    }
    let cancelled = false
    Promise.all([teamApi.getTeam(teamID), teamApi.getTeamWallets(teamID)])
      .then(([teamResponse, walletsResponse]) => {
        if (cancelled) return
        const walletList = walletsResponse.data.data || []
        setTeam(teamResponse.data.data)
        setAccounts(walletList)
        setSelectedID((walletList.find((wallet) => wallet.balance !== 0) || walletList[0])?.id || null)
      })
      .catch((requestError) => {
        if (!cancelled) setError(requestError.response?.data?.error || t('pages.teamWallet.errors.loadFailed'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [teamID, t])

  useEffect(() => {
    const currencyCode = selectedAccount?.currency?.code
    if (!currencyCode) {
      setTransactions([])
      return
    }
    let cancelled = false
    setHistoryLoading(true)
    teamApi.getTeamWalletHistory(teamID, currencyCode)
      .then((response) => {
        if (!cancelled) setTransactions(response.data.data || [])
      })
      .catch((requestError) => {
        if (!cancelled) setError(requestError.response?.data?.error || t('pages.teamWallet.errors.historyFailed'))
      })
      .finally(() => {
        if (!cancelled) setHistoryLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [selectedAccount?.currency?.code, teamID, t])

  if (loading) {
    return <Layout><div className="py-6"><PSkeleton.Content cards={3} /></div></Layout>
  }

  return (
    <Layout>
      <div className="space-y-6">
        <PPageHeader
          title={t('pages.teamWallet.title', { team: team?.name || '' })}
          description={t('pages.teamWallet.description')}
          backTo={`/teams/${teamID}`}
          backLabel={t('pages.teamWallet.backToTeam')}
        />

        {error ? <PAlert variant="error" title={t('pages.teamWallet.errors.title')} message={error} /> : null}

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {accounts.map((account) => {
            const selected = account.id === selectedAccount?.id
            return (
              <button
                key={account.id}
                type="button"
                onClick={() => setSelectedID(account.id)}
                className={`min-h-36 rounded-lg border bg-white p-5 text-left shadow-sm transition-colors ${
                  selected ? 'border-indigo-500 ring-2 ring-indigo-100' : 'border-gray-200 hover:border-indigo-300'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-50">
                    <WalletIcon className="h-5 w-5 text-indigo-600" />
                  </div>
                  <PBadge variant={selected ? 'info' : 'default'}>{account.currency?.type || 'wallet'}</PBadge>
                </div>
                <p className="mt-4 text-sm font-medium text-gray-500">
                  {account.currency?.name_cn || account.currency?.name || account.currency?.code}
                </p>
                <p className="mt-1 break-words text-2xl font-semibold text-gray-950">{formatAmount(account.balance, account)}</p>
              </button>
            )
          })}
        </div>

        {accounts.length === 0 ? (
          <div className="rounded-lg border border-dashed border-gray-300 bg-white px-6 py-10 text-center">
            <BanknotesIcon className="mx-auto h-10 w-10 text-gray-400" />
            <p className="mt-3 text-sm text-gray-600">{t('pages.teamWallet.empty')}</p>
          </div>
        ) : null}

        {selectedAccount ? (
          <PCard padding="none">
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
              <div>
                <h2 className="text-base font-semibold text-gray-950">{t('pages.teamWallet.transactions.title')}</h2>
                <p className="mt-1 text-sm text-gray-500">{selectedAccount.currency?.code}</p>
              </div>
              <PBadge variant="success">{t('pages.teamWallet.readOnly')}</PBadge>
            </div>
            <div className="px-5 py-2">
              {historyLoading ? <div className="py-5"><PSkeleton.List items={3} /></div> : null}
              {!historyLoading && transactions.length === 0 ? (
                <p className="py-8 text-center text-sm text-gray-500">{t('pages.teamWallet.transactions.empty')}</p>
              ) : null}
              {!historyLoading && transactions.length > 0 ? (
                <ul className="divide-y divide-gray-200">
                  {transactions.map((tx) => {
                    const incoming = isIncomingTransaction(tx)
                    return (
                      <li key={tx.id} className="flex items-center gap-4 py-4">
                        <div className={`flex h-9 w-9 flex-none items-center justify-center rounded-full ${incoming ? 'bg-green-50' : 'bg-red-50'}`}>
                          {incoming
                            ? <ArrowUpIcon className="h-4 w-4 text-green-600" />
                            : <ArrowDownIcon className="h-4 w-4 text-red-600" />}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium text-gray-950">{tx.type}</p>
                          <p className="truncate text-xs text-gray-500">
                            {new Date(tx.created_at).toLocaleString(locale)}{tx.reference ? ` · ${tx.reference}` : ''}
                          </p>
                        </div>
                        <div className="text-right">
                          <p className={`text-sm font-semibold ${incoming ? 'text-green-700' : 'text-red-700'}`}>
                            {incoming ? '+' : '-'}{formatAmount(Math.abs(tx.amount), selectedAccount)}
                          </p>
                          <p className="mt-1 text-xs text-gray-500">{tx.status}</p>
                        </div>
                      </li>
                    )
                  })}
                </ul>
              ) : null}
            </div>
          </PCard>
        ) : null}
      </div>
    </Layout>
  )
}
