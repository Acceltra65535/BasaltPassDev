import { useState, useEffect } from 'react'
import { getCurrencies, Currency } from '@api/user/currency'
import { ChevronDownIcon } from '@heroicons/react/24/outline'

interface CurrencySelectorProps {
  value: string
  onChange: (currency: Currency) => void
  className?: string
}

export default function CurrencySelector({ value, onChange, className = '' }: CurrencySelectorProps) {
  const [currencies, setCurrencies] = useState<Currency[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadCurrencies()
  }, [])

  const loadCurrencies = async () => {
    try {
      const response = await getCurrencies()
      setCurrencies(response.data)
      // ，
      if (!value && response.data.length > 0) {
        const defaultCurrency = response.data.find(c => c.code === 'USD') || response.data[0]
        onChange(defaultCurrency)
      }
    } catch (error) {
      console.error('Failed to load currencies:', error)
    } finally {
      setLoading(false)
    }
  }

  const selectedCurrency = currencies.find(c => c.code === value)

  if (loading) {
    return (
      <div className={`h-10 animate-pulse rounded-lg bg-gray-200 ${className}`}></div>
    )
  }

  return (
    <div className={`relative ${className}`}>
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="relative min-h-10 w-full cursor-pointer rounded-lg border border-gray-300 bg-white py-2 pl-3 pr-10 text-left text-sm shadow-sm transition-colors duration-150 hover:border-gray-400 focus:border-indigo-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
      >
        <span className="flex items-center">
          {selectedCurrency ? (
            <>
              <span className="text-lg mr-2">{selectedCurrency.symbol}</span>
              <span className="block truncate">
                {selectedCurrency.name_cn || selectedCurrency.name} ({selectedCurrency.code})
              </span>
            </>
          ) : (
            <span className="block truncate text-gray-400">Select currency</span>
          )}
        </span>
        <span className="pointer-events-none absolute inset-y-0 right-0 ml-3 flex items-center pr-3">
          <ChevronDownIcon className="h-5 w-5 text-gray-400" />
        </span>
      </button>

      {isOpen && (
        <div className="absolute z-10 mt-2 max-h-56 w-full overflow-auto rounded-lg border border-gray-200 bg-white py-1 text-sm shadow-lg focus:outline-none" role="listbox">
          {currencies.map((currency) => (
            <div
              key={currency.id}
              role="option"
              aria-selected={value === currency.code}
              onClick={() => {
                onChange(currency)
                setIsOpen(false)
              }}
              className={`relative cursor-pointer select-none py-2 pl-3 pr-9 transition-colors hover:bg-gray-100 ${
                value === currency.code ? 'bg-indigo-50 text-indigo-700' : 'text-gray-900'
              }`}
            >
              <div className="flex items-center">
                <span className="text-lg mr-2">{currency.symbol}</span>
                <span className="block truncate">
                  {currency.name_cn || currency.name} ({currency.code})
                </span>
                {currency.type === 'crypto' && (
                  <span className="ml-2 inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-yellow-100 text-yellow-800">
                    Crypto
                  </span>
                )}
                {currency.type === 'points' && (
                  <span className="ml-2 inline-flex items-center rounded px-2 py-0.5 text-xs font-medium bg-indigo-100 text-indigo-800">
                    Points
                  </span>
                )}
              </div>
              {currency.description && (
                <p className="text-gray-500 text-xs mt-1 ml-7">{currency.description}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
