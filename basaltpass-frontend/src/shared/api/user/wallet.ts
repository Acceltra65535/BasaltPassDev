import client from '../client'
import type { Currency } from './currency'

export interface WalletAccount {
  ID: number
  TenantID: number
  UserID?: number
  CurrencyID?: number
  Balance: number
  Freeze: number
  Currency?: Currency | null
}

export interface WalletTransaction {
  ID: number
  Type: string
  Amount: number
  Status: string
  CreatedAt: string
  Reference?: string
  Wallet?: WalletAccount | null
}

export const getBalance = (currency: string) => client.get('/api/v1/wallet/balance', { params: { currency } })
export const getAccounts = () => client.get<WalletAccount[]>('/api/v1/wallet/accounts')
export const recharge = (currency: string, amount: number) => client.post('/api/v1/wallet/recharge', { currency, amount })
export const withdraw = (currency: string, amount: number) => client.post('/api/v1/wallet/withdraw', { currency, amount })
export const history = (currency?: string, limit = 20) =>
  client.get<WalletTransaction[]>('/api/v1/wallet/history', {
    params: {
      ...(currency ? { currency } : {}),
      limit,
    },
  })
