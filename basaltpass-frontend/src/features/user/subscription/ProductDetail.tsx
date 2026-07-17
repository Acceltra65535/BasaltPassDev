import React, { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import Layout from '@features/user/components/Layout'
import { PBadge, PButton, PCard, PEmptyState, PPageHeader, PSkeleton } from '@ui'
import { subscriptionAPI } from '@api/subscription/subscription'
import { createOrder, CreateOrderRequest } from '@api/subscription/payment/order'
import client from '@api/client'
import { Product, Plan, Price } from '@types/domain/subscription'
import { ROUTES } from '@constants'
import { useI18n } from '@shared/i18n'
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  CheckIcon,
  CreditCardIcon,
  CubeIcon,
  SparklesIcon,
} from '@heroicons/react/24/outline'

function unwrapProduct(raw: any): Product | null {
  if (!raw) return null
  if (raw.ID || raw.id) return raw as Product
  if (raw.data?.ID || raw.data?.id) return raw.data as Product
  if (raw.data?.data?.ID || raw.data?.data?.id) return raw.data.data as Product
  return null
}

function unwrapProducts(raw: any): Product[] {
  if (Array.isArray(raw)) return raw as Product[]
  if (Array.isArray(raw?.data)) return raw.data as Product[]
  if (Array.isArray(raw?.data?.data)) return raw.data.data as Product[]
  if (Array.isArray(raw?.data?.Data)) return raw.data.Data as Product[]
  return []
}

function priceId(price: Price): number {
  return price.ID ?? (price as any).id
}

function planId(plan: Plan): number {
  return plan.ID ?? (plan as any).id
}

export default function ProductDetailPage() {
  const { t } = useI18n()
  const { productId = '', productCode = '' } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [product, setProduct] = useState<Product | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedPriceId, setSelectedPriceId] = useState<number | null>(null)
  const [subscribingPrice, setSubscribingPrice] = useState<number | null>(null)

  useEffect(() => {
    const loadProduct = async () => {
      const numericProductId = Number(productId)
      if (!productCode && (!Number.isFinite(numericProductId) || numericProductId <= 0)) {
        setError('Invalid product link')
        setLoading(false)
        return
      }

      try {
        setLoading(true)
        setError(null)
        const raw = productCode
          ? await client.get('/api/v1/products', { params: { code: productCode, page_size: 50 } })
          : await subscriptionAPI.getProduct(numericProductId)
        const nextProduct = productCode
          ? unwrapProducts(raw.data).find((item) => item.Code === productCode) || null
          : unwrapProduct(raw)
        if (!nextProduct) {
          setError('Product not found')
          return
        }
        setProduct(nextProduct)
      } catch (err: any) {
        console.error('failed to load product:', err)
        setError(err.response?.data?.error || err.message || 'Failed to load product')
      } finally {
        setLoading(false)
      }
    }

    loadProduct()
  }, [productCode, productId])

  const availablePrices = useMemo(() => {
    return (product?.Plans || []).flatMap((plan) =>
      (plan.Prices || []).map((price) => ({ plan, price })),
    )
  }, [product])

  useEffect(() => {
    if (availablePrices.length === 0 || selectedPriceId !== null) return
    const requestedPriceId = Number(searchParams.get('price_id') || searchParams.get('price'))
    const requestedPlanId = Number(searchParams.get('plan_id') || searchParams.get('plan'))
    const exactPrice = availablePrices.find(({ price }) => priceId(price) === requestedPriceId)
    const planFirstPrice = availablePrices.find(({ plan }) => planId(plan) === requestedPlanId)
    const initialPrice = exactPrice || planFirstPrice || availablePrices[0]
    setSelectedPriceId(priceId(initialPrice.price))
  }, [availablePrices, searchParams, selectedPriceId])

  const selectedEntry = availablePrices.find(({ price }) => priceId(price) === selectedPriceId)

  const formatPrice = (price: Price) => {
    const amount = (price.AmountCents / 100).toFixed(2)
    const period = price.BillingPeriod === 'month' ? t('pages.userSubscriptionProducts.period.month') :
                   price.BillingPeriod === 'year' ? t('pages.userSubscriptionProducts.period.year') :
                   price.BillingPeriod === 'week' ? t('pages.userSubscriptionProducts.period.week') :
                   price.BillingPeriod === 'day' ? t('pages.userSubscriptionProducts.period.day') : price.BillingPeriod
    const interval = price.BillingInterval > 1 ? `${price.BillingInterval}` : ''
    return `¥${amount}/${interval}${period}`
  }

  const usageText = (price: Price) => {
    if (price.UsageType === 'license') return t('pages.userSubscriptionProducts.usage.license')
    if (price.UsageType === 'metered') return t('pages.userSubscriptionProducts.usage.metered')
    return t('pages.userSubscriptionProducts.usage.tiered')
  }

  const handleSubscribe = async () => {
    if (!selectedEntry) return

    const selectedPrice = selectedEntry.price
    const selectedPriceID = priceId(selectedPrice)
    try {
      setSubscribingPrice(selectedPriceID)
      const userResponse = await client.get('/api/v1/user/profile')
      const user = userResponse.data
      const orderData: CreateOrderRequest = {
        user_id: user.id,
        price_id: selectedPriceID,
        quantity: 1,
      }
      const order = await createOrder(orderData)
      navigate(`/orders/${order.id}/confirm`)
    } catch (err: any) {
      console.error('failed to create order:', err)
      if (err.response?.status === 401) {
        navigate(ROUTES.user.login)
        return
      }
      setError(err.response?.data?.error || err.message || t('pages.userSubscriptionProducts.errors.createOrderFailed'))
    } finally {
      setSubscribingPrice(null)
    }
  }

  if (loading) {
    return (
      <Layout>
        <div className="space-y-6">
          <PSkeleton.AppCardGrid count={3} />
        </div>
      </Layout>
    )
  }

  if (error || !product) {
    return (
      <Layout>
        <PEmptyState
          icon={<CubeIcon className="h-12 w-12 text-gray-300" />}
          title={error || 'Product not found'}
          description="Please check whether the service link is still valid."
          action={{ label: 'Browse products', onClick: () => navigate(ROUTES.user.products) }}
        />
      </Layout>
    )
  }

  return (
    <Layout>
      <div className="space-y-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <PPageHeader title={product.Name} description={product.Description || 'Choose a plan for this service'} />
          <div className="flex flex-wrap gap-2">
            <Link to={ROUTES.user.products}>
              <PButton variant="secondary" leftIcon={<ArrowLeftIcon />}>
                All products
              </PButton>
            </Link>
            <Link to={ROUTES.user.subscriptions}>
              <PButton variant="secondary" leftIcon={<CreditCardIcon />}>
                {t('pages.userSubscriptionProducts.actions.mySubscriptions')}
              </PButton>
            </Link>
          </div>
        </div>

        <PCard variant="bordered" className="p-6">
          <div className="flex flex-col gap-4 border-b border-gray-200 pb-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-center">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-indigo-50">
                <CubeIcon className="h-7 w-7 text-indigo-600" />
              </div>
              <div className="ml-4">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-xl font-semibold text-gray-900">{product.Name}</h2>
                  <PBadge variant={product.IsActive === false ? 'warning' : 'success'}>
                    {product.IsActive === false ? 'Inactive' : 'Available'}
                  </PBadge>
                </div>
                <p className="mt-1 text-sm text-gray-500">
                  {t('pages.userSubscriptionProducts.productCode')}: {product.Code}
                </p>
              </div>
            </div>
            {selectedEntry && (
              <div className="rounded-lg border border-indigo-200 bg-indigo-50 px-4 py-3 text-right">
                <p className="text-sm text-indigo-700">Selected</p>
                <p className="text-lg font-semibold text-indigo-900">{formatPrice(selectedEntry.price)}</p>
              </div>
            )}
          </div>

          {availablePrices.length > 0 ? (
            <div className="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
              <div className="space-y-4">
                {(product.Plans || []).map((plan) => (
                  <div key={plan.ID} className="rounded-lg border border-gray-200 bg-white p-4">
                    <div className="mb-4 flex items-center justify-between">
                      <div>
                        <h3 className="text-lg font-medium text-gray-900">{plan.DisplayName}</h3>
                        <p className="text-sm text-gray-500">{t('pages.userSubscriptionProducts.version')} v{plan.PlanVersion}</p>
                      </div>
                      <SparklesIcon className="h-5 w-5 text-indigo-600" />
                    </div>

                    {plan.Features && plan.Features.length > 0 && (
                      <div className="mb-4 grid gap-2 sm:grid-cols-2">
                        {plan.Features.map((feature) => (
                          <div key={feature.ID} className="flex items-start text-sm text-gray-600">
                            <CheckIcon className="mr-2 mt-0.5 h-4 w-4 shrink-0 text-green-500" />
                            <span>
                              <span className="font-medium text-gray-800">{feature.FeatureKey}</span>
                              {feature.IsUnlimited ? (
                                <span className="ml-1 text-gray-500">({t('pages.userSubscriptionProducts.unlimited')})</span>
                              ) : (
                                <>
                                  {feature.ValueText && <span className="ml-1">: {feature.ValueText}</span>}
                                  {feature.ValueNumeric !== undefined && <span className="ml-1">: {feature.ValueNumeric}</span>}
                                  {feature.Unit && <span className="ml-1">{feature.Unit}</span>}
                                </>
                              )}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}

                    <div className="grid gap-3 sm:grid-cols-2">
                      {(plan.Prices || []).map((price) => {
                        const currentPriceId = priceId(price)
                        const selected = selectedPriceId === currentPriceId
                        return (
                          <button
                            key={price.ID}
                            type="button"
                            onClick={() => setSelectedPriceId(currentPriceId)}
                            className={`min-h-24 rounded-lg border p-4 text-left transition-colors ${
                              selected
                                ? 'border-indigo-500 bg-indigo-50 ring-2 ring-indigo-100'
                                : 'border-gray-200 bg-gray-50 hover:border-gray-300'
                            }`}
                          >
                            <div className="flex items-start justify-between gap-3">
                              <div>
                                <p className="text-lg font-semibold text-gray-900">{formatPrice(price)}</p>
                                <p className="mt-1 text-sm text-gray-500">{usageText(price)}</p>
                              </div>
                              {selected && <PBadge variant="info">Selected</PBadge>}
                            </div>
                            {price.TrialDays && price.TrialDays > 0 && (
                              <p className="mt-2 text-sm text-green-700">
                                {t('pages.userSubscriptionProducts.trialDays', { days: price.TrialDays })}
                              </p>
                            )}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                ))}
              </div>

              <div className="lg:sticky lg:top-6 lg:self-start">
                <PCard variant="bordered" className="p-5">
                  <h3 className="text-base font-semibold text-gray-900">Purchase service</h3>
                  {selectedEntry ? (
                    <div className="mt-4 space-y-4">
                      <div>
                        <p className="text-sm text-gray-500">Plan</p>
                        <p className="font-medium text-gray-900">{selectedEntry.plan.DisplayName}</p>
                      </div>
                      <div>
                        <p className="text-sm text-gray-500">Price</p>
                        <p className="text-2xl font-semibold text-gray-900">{formatPrice(selectedEntry.price)}</p>
                      </div>
                      <PButton
                        fullWidth
                        onClick={handleSubscribe}
                        loading={subscribingPrice === priceId(selectedEntry.price)}
                        rightIcon={<ArrowRightIcon />}
                      >
                        {t('pages.userSubscriptionProducts.actions.subscribeNow')}
                      </PButton>
                    </div>
                  ) : (
                    <p className="mt-3 text-sm text-gray-500">{t('pages.userSubscriptionProducts.noPrice')}</p>
                  )}
                </PCard>
              </div>
            </div>
          ) : (
            <PEmptyState
              icon={<CreditCardIcon className="h-12 w-12 text-gray-300" />}
              title={t('pages.userSubscriptionProducts.noPrice')}
              description="This product is visible, but no active price is available yet."
            />
          )}
        </PCard>
      </div>
    </Layout>
  )
}
