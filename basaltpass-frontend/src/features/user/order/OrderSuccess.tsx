import React, { useState, useEffect } from 'react'
import { useParams, Link, useLocation } from 'react-router-dom'
import Layout from '@features/user/components/Layout'
import { getOrder } from '@api/subscription/payment/order'
import { OrderResponse } from '@api/subscription/payment/order'
import { CheckCircleIcon } from '@heroicons/react/24/solid'
import { CreditCardIcon, DocumentTextIcon, HomeIcon } from '@heroicons/react/24/outline'
import { ROUTES } from '@constants'
import { useI18n } from '@shared/i18n'
import { PAlert, PButton, PCard } from '@ui'

export default function OrderSuccessPage() {
  const { t, locale } = useI18n()
  const { orderId } = useParams<{ orderId: string }>()
  const location = useLocation()
  const [order, setOrder] = useState<OrderResponse | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (orderId) {
      fetchOrder(parseInt(orderId))
    }
  }, [orderId])

  const fetchOrder = async (id: number) => {
    try {
      setLoading(true)
      const search = new URLSearchParams(location.search)
      const shouldActivate = search.get('paid') === '1' || search.get('activate') === '1'
      const orderData = await getOrder(id, shouldActivate ? { activate: true } : undefined)
      setOrder(orderData)
    } catch (error: any) {
      console.error(t('userOrderSuccess.logs.fetchOrderFailed'), error)
    } finally {
      setLoading(false)
    }
  }

  const formatAmount = (cents: number) => {
    return (cents / 100).toFixed(2)
  }

  if (loading) {
    return (
      <Layout>
        <div className="flex justify-center items-center h-64">
          <div className="text-lg">{t('userOrderSuccess.loading')}</div>
        </div>
      </Layout>
    )
  }

  return (
    <Layout>
      <div className="min-h-screen bg-gray-50 flex flex-col justify-center py-12 sm:px-6 lg:px-8">
        <div className="sm:mx-auto sm:w-full sm:max-w-md">
          {/*  */}
          <div className="mx-auto flex items-center justify-center h-20 w-20 rounded-full bg-green-100 mb-6">
            <CheckCircleIcon className="h-12 w-12 text-green-600" />
          </div>
          
          {/*  */}
          <div className="text-center">
            <h1 className="text-3xl font-bold text-gray-900 mb-2">{t('userOrderSuccess.title')}</h1>
            <p className="text-lg text-gray-600">{t('userOrderSuccess.subtitle')}</p>
          </div>
        </div>

        {order && (
          <div className="mt-8 sm:mx-auto sm:w-full sm:max-w-lg">
            <PCard padding="lg" className="sm:px-10">
              {/*  */}
              <div className="mb-6">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="text-lg font-medium text-gray-900">{t('userOrderSuccess.orderDetail.title')}</h3>
                  <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    {t('userOrderSuccess.orderDetail.paidStatus')}
                  </span>
                </div>
                
                <div className="bg-gray-50 rounded-lg p-4 space-y-3">
                  <div className="flex justify-between items-center">
                    <span className="text-sm text-gray-600">{t('userOrderSuccess.orderDetail.orderNumber')}</span>
                    <span className="text-sm font-mono text-gray-900">{order.order_number}</span>
                  </div>
                  
                  {order.price && (
                    <div className="flex justify-between items-start">
                      <span className="text-sm text-gray-600">{t('userOrderSuccess.orderDetail.product')}</span>
                      <div className="text-right">
                        <div className="text-sm font-medium text-gray-900">
                          {order.price.plan?.product?.name}
                        </div>
                        <div className="text-xs text-gray-500">
                          {order.price.plan?.display_name}
                        </div>
                      </div>
                    </div>
                  )}
                  
                  <div className="flex justify-between items-center">
                    <span className="text-sm text-gray-600">{t('userOrderSuccess.orderDetail.amount')}</span>
                    <span className="text-lg font-semibold text-gray-900">¥{formatAmount(order.total_amount)}</span>
                  </div>
                  
                  {order.paid_at && (
                    <div className="flex justify-between items-center">
                      <span className="text-sm text-gray-600">{t('userOrderSuccess.orderDetail.paidAt')}</span>
                      <span className="text-sm text-gray-900">{new Date(order.paid_at).toLocaleString(locale)}</span>
                    </div>
                  )}
                </div>
              </div>

              {/*  */}
              <PAlert className="mb-6" variant="info" title={t('userOrderSuccess.nextStep.title')} message={t('userOrderSuccess.nextStep.description')} />

              {/*  */}
              <div className="space-y-3">
                <Link to={ROUTES.user.subscriptions}>
                  <PButton fullWidth leftIcon={<CreditCardIcon className="h-4 w-4" />}>
                    {t('userOrderSuccess.actions.viewSubscriptions')}
                  </PButton>
                </Link>
                
                <Link to={ROUTES.user.dashboard}>
                  <PButton fullWidth variant="secondary" leftIcon={<HomeIcon className="h-4 w-4" />}>
                    {t('userOrderSuccess.actions.backToConsole')}
                  </PButton>
                </Link>
                
                <Link to={ROUTES.user.products}>
                  <PButton fullWidth variant="secondary" leftIcon={<DocumentTextIcon className="h-4 w-4" />}>
                    {t('userOrderSuccess.actions.viewMoreProducts')}
                  </PButton>
                </Link>
              </div>
            </PCard>
            
            {/*  */}
            <div className="mt-6 text-center">
              <p className="text-sm text-gray-500">
                {t('userOrderSuccess.footer.prefix')}
                <Link to={ROUTES.user.help} className="font-medium text-indigo-600 hover:text-indigo-500">
                  {t('userOrderSuccess.footer.helpCenter')}
                </Link>
                {t('userOrderSuccess.footer.suffix')}
              </p>
            </div>
          </div>
        )}
      </div>
    </Layout>
  )
}
