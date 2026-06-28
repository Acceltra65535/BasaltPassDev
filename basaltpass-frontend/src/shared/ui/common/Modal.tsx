import React from 'react'
import { XMarkIcon } from '@heroicons/react/24/outline'

interface ModalProps {
  open: boolean
  title: string
  onClose: () => void
  children: React.ReactNode
  footer?: React.ReactNode
  widthClass?: string
  description?: React.ReactNode
}

const Modal: React.FC<ModalProps> = ({
  open,
  title,
  onClose,
  children,
  footer,
  widthClass = 'max-w-lg',
  description
}) => {
  if (!open) {
    return null
  }

  return (
    <div className="fixed inset-0 !m-0 z-50 flex items-center justify-center">
      <div
        className="fixed inset-0 !m-0 bg-gray-900/45 backdrop-blur-[2px]"
        onClick={onClose}
        aria-hidden="true"
      ></div>
      <div className={`relative mx-4 w-full rounded-2xl bg-white shadow-lg dark:bg-gray-900 dark:shadow-black/30 ${widthClass}`}>
        <div className="flex items-start justify-between border-b border-gray-200 px-6 py-5 dark:border-white/10">
          <div>
            <h3 className="text-base font-semibold text-gray-900 dark:text-gray-50">{title}</h3>
            {description && <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{description}</p>}
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1 text-gray-400 transition hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 dark:hover:bg-white/10 dark:hover:text-gray-200 dark:focus:ring-offset-gray-900"
            aria-label="translated"
          >
            <XMarkIcon className="w-5 h-5" />
          </button>
        </div>
        <div className="px-6 py-4 max-h-[70vh] overflow-y-auto text-gray-900 dark:text-gray-100">{children}</div>
        {footer && <div className="border-t border-gray-100 bg-gray-50 px-6 py-4 rounded-b-2xl dark:border-white/10 dark:bg-gray-950">{footer}</div>}
      </div>
    </div>
  )
}

export default Modal
