import client from '../client'
import type { Currency } from './currency'

export interface AppRechargeApp {
  id: number
  tenant_id: number
  name: string
  description?: string
  icon_url?: string
  logo_url?: string
  homepage_url?: string
}

export interface AppRechargeCurrency extends Currency {
  wallet_category?: string
  is_default?: boolean
}

export interface AppRechargeConfig {
  app: AppRechargeApp
  wallet_category: string
  currencies: AppRechargeCurrency[]
}

export const getAppRechargeConfig = (params: { app_id?: string | number; client_id?: string; category?: string; tenant?: string }) =>
  client.get<AppRechargeConfig>('/api/v1/user/apps/recharge-config', { params })
