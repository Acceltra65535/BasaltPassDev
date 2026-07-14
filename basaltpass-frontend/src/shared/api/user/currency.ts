import client from '../client'

export interface Currency {
  id: number
  code: string
  name: string
  name_cn: string
  symbol: string
  decimal_places: number
  type: string
  exchange_rate_usd?: number
  payment_enabled?: boolean
  is_active: boolean
  sort_order: number
  description: string
  icon_url?: string
}

export interface CurrencyRate {
  id: number
  base_currency_code: string
  quote_currency_code: string
  rate: number
  source: string
  is_active: boolean
  description?: string
}

export const getCurrencies = () => client.get<Currency[]>('/api/v1/currencies')
export const getCurrency = (code: string) => client.get<Currency>(`/api/v1/currencies/${code}`)
export const getCurrencyRates = () => client.get<CurrencyRate[]>('/api/v1/currencies/rates')
