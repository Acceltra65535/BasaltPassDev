import React from 'react'
import Layout from '@features/user/components/Layout'
import { PCard, PPageHeader } from '@ui'
import { ROUTES } from '@constants'
import { Link } from 'react-router-dom'
import { buildInfo } from '@shared/generated/buildInfo'
import { useI18n } from '@shared/i18n'

const About: React.FC = () => {
  const currentYear = new Date().getFullYear()
  const { t } = useI18n()

  return (
    <Layout>
      <div className="max-w-4xl mx-auto space-y-8">
        <PPageHeader
          title="BasaltPass"
          description={t('pages.about.subtitle')}
          backTo={ROUTES.user.dashboard}
          backLabel={t('pages.about.back')}
        />

        <PCard>
          <h2 className="text-lg font-semibold text-gray-900 mb-4">{t('pages.about.coreFeaturesTitle')}</h2>
          <ul className="space-y-2 text-gray-600">
            <li>• {t('pages.about.coreFeatures.teamCollaboration')}</li>
            <li>• {t('pages.about.coreFeatures.multiCurrencyWallet')}</li>
            <li>• {t('pages.about.coreFeatures.securityAuth')}</li>
            <li>• {t('pages.about.coreFeatures.notificationSystem')}</li>
            <li>• {t('pages.about.coreFeatures.adminPanel')}</li>
            <li>• {t('pages.about.coreFeatures.subscriptionSystem')}</li>
          </ul>
        </PCard>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <PCard>
            <h3 className="font-semibold text-gray-900 mb-3">{t('pages.about.frontendTechTitle')}</h3>
            <ul className="space-y-1 text-sm text-gray-600">
              <li>React 18 + TypeScript</li>
              <li>Tailwind CSS</li>
              <li>Vite + React Router</li>
              <li>Axios</li>
            </ul>
          </PCard>

          <PCard>
            <h3 className="font-semibold text-gray-900 mb-3">{t('pages.about.backendTechTitle')}</h3>
            <ul className="space-y-1 text-sm text-gray-600">
              <li>Go Fiber + GORM</li>
              <li>SQLite</li>
              <li>JWT + OAuth2</li>
              <li>bcrypt</li>
            </ul>
          </PCard>
        </div>

        <PCard>
          <h2 className="text-lg font-semibold text-gray-900 mb-4">{t('pages.about.versionInfoTitle')}</h2>
          <div className="grid grid-cols-3 gap-6 text-center">
            <div>
              <div className="text-2xl font-bold text-gray-900">{buildInfo.version}</div>
              <div className="text-sm text-gray-500">{t('pages.about.versionInfo.currentVersion')}</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-gray-900">{currentYear}</div>
              <div className="text-sm text-gray-500">{t('pages.about.versionInfo.releaseYear')}</div>
            </div>
            <div>
              <div className="text-2xl font-bold text-gray-900">MIT</div>
              <div className="text-sm text-gray-500">{t('pages.about.versionInfo.openSourceLicense')}</div>
            </div>
          </div>
        </PCard>

        <div className="text-center text-sm text-gray-600 pb-8">
          <Link to={ROUTES.user.copyright} className="text-indigo-600 hover:underline">
            {t('pages.about.links.softwareCopyright')}
          </Link>
          {' · '}
          <a href="https://github.com/BasaltBase/BasaltPass" className="text-indigo-600 hover:underline">
            GitHub
          </a>
          {' · '}
          <a href="HollowData.com" className="text-indigo-600 hover:underline">
            {t('pages.about.links.contactUs')}
          </a>
        </div>
      </div>
    </Layout>
  )
}

export default About 
