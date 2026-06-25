import { ArrowPathIcon, ShieldCheckIcon } from '@heroicons/react/24/outline'

interface OAuthRedirectingProps {
  appName?: string
}

export default function OAuthRedirecting({ appName }: OAuthRedirectingProps) {
  const destination = appName?.trim() || 'the application'

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-md rounded-lg bg-white px-8 py-10 text-center shadow-sm ring-1 ring-gray-200">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-indigo-50">
          <ShieldCheckIcon className="h-8 w-8 text-indigo-600" aria-hidden="true" />
        </div>
        <h1 className="mt-6 text-xl font-semibold text-gray-900">Authorization complete</h1>
        <p className="mt-3 text-sm leading-6 text-gray-600">
          Redirecting back to {destination}. Keep this page open.
        </p>
        <div className="mt-8 flex items-center justify-center gap-2 text-sm font-medium text-indigo-600">
          <ArrowPathIcon className="h-5 w-5 animate-spin" aria-hidden="true" />
          <span>Redirecting</span>
        </div>
      </div>
    </div>
  )
}
