import { Route } from 'react-router-dom'
import UserAppsIndex from '@pages/user/apps/Index'
import UserAppDetail from '@pages/user/apps/Detail'
import AppRecharge from '@pages/user/apps/AppRecharge'
import { withProtected } from '@/routes/helpers'

export function UserAppRoutes() {
  return (
    <>
      <Route path="/my-apps" element={withProtected(<UserAppsIndex />)} />
      <Route path="/my-apps/:id" element={withProtected(<UserAppDetail />)} />
      <Route path="/apps/recharge" element={withProtected(<AppRecharge />)} />
      <Route path="/apps/:id/recharge" element={withProtected(<AppRecharge />)} />
    </>
  )
}
