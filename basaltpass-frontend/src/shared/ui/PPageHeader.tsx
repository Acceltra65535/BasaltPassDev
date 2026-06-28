import React from 'react'
import { ChevronLeftIcon } from '@heroicons/react/24/outline'
import { Link } from 'react-router-dom'

interface PPageHeaderProps {
  /** translated */
  title: React.ReactNode
  /** translated/description */
  description?: React.ReactNode
  /** translated */
  icon?: React.ReactNode
  /** translated（translatedisPButtontranslated） */
  actions?: React.ReactNode
  /** backtranslated（translated react-router Link） */
  backTo?: string
  /** backtranslated */
  backLabel?: string
  /** backtranslated（and backTo translated） */
  onBack?: () => void
  className?: string
}

const PPageHeader: React.FC<PPageHeaderProps> = ({
  title,
  description,
  icon,
  actions,
  backTo,
  backLabel = 'back',
  onBack,
  className = '',
}) => {
  const showBack = backTo || onBack

  return (
    <div className={`mb-6 ${className}`}>
      {showBack && (
        <div className="mb-3">
          {backTo ? (
            <Link
              to={backTo}
              className="inline-flex min-h-8 items-center gap-1 rounded-lg px-2.5 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 dark:text-gray-400 dark:hover:bg-white/10 dark:hover:text-gray-200"
            >
              <ChevronLeftIcon className="h-4 w-4" />
              {backLabel}
            </Link>
          ) : (
            <button
              type="button"
              onClick={onBack}
              className="inline-flex min-h-8 items-center gap-1 rounded-lg px-2.5 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 dark:text-gray-400 dark:hover:bg-white/10 dark:hover:text-gray-200"
            >
              <ChevronLeftIcon className="h-4 w-4" />
              {backLabel}
            </button>
          )}
        </div>
      )}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          {icon && (
            <div className="flex-shrink-0 text-gray-600 dark:text-gray-300">
              {icon}
            </div>
          )}
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-gray-900 truncate dark:text-gray-50">
              {title}
            </h1>
            {description && (
              <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{description}</p>
            )}
          </div>
        </div>
        {actions && (
          <div className="flex-shrink-0 flex flex-wrap items-center gap-2">
            {actions}
          </div>
        )}
      </div>
    </div>
  )
}

PPageHeader.displayName = 'PPageHeader'

export default PPageHeader
