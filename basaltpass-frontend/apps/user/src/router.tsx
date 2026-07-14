import { Routes, Route, Navigate } from 'react-router-dom'
import ProtectedRoute from '@routes/ProtectedRoute'
import PublicRoute from '@routes/PublicRoute'
import Login from '../../../src/features/auth/Login'
import Register from '../../../src/features/auth/Register'
import TenantLogin from '../../../src/features/auth/TenantLogin'
import TenantRegister from '../../../src/features/auth/TenantRegister'
import TenantJoin from '../../../src/features/auth/TenantJoin'
import OauthSuccess from '../../../src/features/auth/OauthSuccess'
import OAuthConsent from '../../../src/features/auth/OAuthConsent'
import ResetPassword from '../../../src/features/auth/ResetPassword'
import Terms from '../../../src/features/auth/Terms'
import Privacy from '../../../src/features/auth/Privacy'

import Dashboard from '../../../src/features/user/Dashboard'
import Profile from '../../../src/features/user/profile/Index'
import Settings from '../../../src/features/user/Settings'
import About from '../../../src/features/user/About'
import SoftwareCopyright from '../../../src/features/user/SoftwareCopyright'
import Notifications from '../../../src/features/user/Notifications'

import TeamIndex from '../../../src/features/user/team/Index'
import CreateTeam from '../../../src/features/user/team/Create'
import TeamDetail from '../../../src/features/user/team/Detail'
import TeamMembers from '../../../src/features/user/team/Members'
import EditTeam from '../../../src/features/user/team/Edit'
import InviteTeam from '../../../src/features/user/team/Invite'
import InvitationInbox from '../../../src/features/user/invitations/Inbox'

import WalletIndex from '../../../src/features/user/wallet/Index'
import Recharge from '../../../src/features/user/wallet/Recharge'
import Withdraw from '../../../src/features/user/wallet/Withdraw'
import History from '../../../src/features/user/wallet/History'
import RedeemGiftCard from '../../../src/features/user/wallet/RedeemGiftCard'
import Payment from '../../../src/features/user/payment/Payment'
import Cashier from '../../../src/features/user/payment/Cashier'

import SecuritySettings from '../../../src/features/user/security/SecuritySettings'
import TwoFA from '../../../src/features/user/security/TwoFA'
import PasskeyManagement from '../../../src/features/user/security/PasskeyManagement'
import LoginHistory from '../../../src/features/user/security/LoginHistory'

import UserAppsIndex from '../../../src/features/user/apps/Index'
import UserAppDetail from '../../../src/features/user/apps/Detail'
import AppRecharge from '../../../src/features/user/apps/AppRecharge'

import SubscriptionIndex from '../../../src/features/user/subscription/Index'
import ProductsPage from '../../../src/features/user/subscription/Products'
import ProductDetailPage from '../../../src/features/user/subscription/ProductDetail'
import SubscriptionCheckout from '../../../src/features/user/subscription/Checkout'

import OrderConfirm from '../../../src/features/user/order/OrderConfirm'
import OrderSuccess from '../../../src/features/user/order/OrderSuccess'
import OrderList from '../../../src/features/user/order/OrderList'

import NotFound from '../../../src/features/NotFound'

export default function AppRouter() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/dashboard" replace />} />

      {/* Public auth */}
      <Route
        path="/login"
        element={
          <PublicRoute>
            <Login />
          </PublicRoute>
        }
      />
      <Route
        path="/register"
        element={
          <PublicRoute>
            <Register />
          </PublicRoute>
        }
      />
      <Route
        path="/auth/tenant/:tenantCode/login"
        element={
          <PublicRoute>
            <TenantLogin />
          </PublicRoute>
        }
      />
      <Route
        path="/auth/tenant/:tenantCode/register"
        element={
          <PublicRoute>
            <TenantRegister />
          </PublicRoute>
        }
      />
      <Route
        path="/auth/tenant/:tenantCode/join"
        element={
          <PublicRoute>
            <TenantJoin />
          </PublicRoute>
        }
      />
      {/* Backward compatibility */}
      <Route
        path="/tenant/:tenantCode/login"
        element={
          <PublicRoute>
            <TenantLogin />
          </PublicRoute>
        }
      />
      <Route
        path="/tenant/:tenantCode/register"
        element={
          <PublicRoute>
            <TenantRegister />
          </PublicRoute>
        }
      />
      <Route
        path="/tenant/:tenantCode/join"
        element={
          <PublicRoute>
            <TenantJoin />
          </PublicRoute>
        }
      />
      <Route path="/oauth-success" element={<OauthSuccess />} />
      <Route path="/oauth-consent" element={<OAuthConsent />} />
      <Route path="/reset-password" element={<ResetPassword />} />
      <Route path="/terms" element={<Terms />} />
      <Route path="/privacy" element={<Privacy />} />

      {/* User console */}
      <Route
        path="/dashboard"
        element={
          <ProtectedRoute>
            <Dashboard />
          </ProtectedRoute>
        }
      />
      <Route
        path="/profile"
        element={
          <ProtectedRoute>
            <Profile />
          </ProtectedRoute>
        }
      />
      <Route
        path="/settings"
        element={
          <ProtectedRoute>
            <Settings />
          </ProtectedRoute>
        }
      />
      <Route
        path="/about"
        element={
          <ProtectedRoute>
            <About />
          </ProtectedRoute>
        }
      />
      <Route
        path="/copyright"
        element={
          <ProtectedRoute>
            <SoftwareCopyright />
          </ProtectedRoute>
        }
      />
      <Route
        path="/notifications"
        element={
          <ProtectedRoute>
            <Notifications />
          </ProtectedRoute>
        }
      />

      {/* Teams */}
      <Route path="/teams" element={<ProtectedRoute><TeamIndex /></ProtectedRoute>} />
      <Route path="/teams/create" element={<ProtectedRoute><CreateTeam /></ProtectedRoute>} />
      <Route path="/teams/:id" element={<ProtectedRoute><TeamDetail /></ProtectedRoute>} />
      <Route path="/teams/:id/members" element={<ProtectedRoute><TeamMembers /></ProtectedRoute>} />
      <Route path="/teams/:id/edit" element={<ProtectedRoute><EditTeam /></ProtectedRoute>} />
      <Route path="/teams/invite/:id" element={<ProtectedRoute><InviteTeam /></ProtectedRoute>} />
      <Route path="/invitations/inbox" element={<ProtectedRoute><InvitationInbox /></ProtectedRoute>} />

      {/* Wallet */}
      <Route path="/wallet" element={<ProtectedRoute requiresTenant><WalletIndex /></ProtectedRoute>} />
      <Route path="/wallet/recharge" element={<ProtectedRoute requiresTenant><Recharge /></ProtectedRoute>} />
      <Route path="/wallet/withdraw" element={<ProtectedRoute requiresTenant><Withdraw /></ProtectedRoute>} />
      <Route path="/wallet/history" element={<ProtectedRoute requiresTenant><History /></ProtectedRoute>} />
      <Route path="/wallet/gift-cards/redeem" element={<ProtectedRoute requiresTenant><RedeemGiftCard /></ProtectedRoute>} />
      <Route path="/payment" element={<ProtectedRoute><Payment /></ProtectedRoute>} />
      <Route path="/checkout" element={<ProtectedRoute><Cashier /></ProtectedRoute>} />

      {/* Security */}
      <Route path="/security" element={<ProtectedRoute><SecuritySettings /></ProtectedRoute>} />
      <Route path="/security/2fa" element={<ProtectedRoute><TwoFA /></ProtectedRoute>} />
      <Route path="/security/passkeys" element={<ProtectedRoute><PasskeyManagement /></ProtectedRoute>} />
      <Route path="/security/login-history" element={<ProtectedRoute><LoginHistory /></ProtectedRoute>} />

      {/* Apps */}
      <Route path="/my-apps" element={<ProtectedRoute><UserAppsIndex /></ProtectedRoute>} />
      <Route path="/my-apps/:id" element={<ProtectedRoute><UserAppDetail /></ProtectedRoute>} />
      <Route path="/apps/recharge" element={<ProtectedRoute requiresTenant><AppRecharge /></ProtectedRoute>} />
      <Route path="/apps/:id/recharge" element={<ProtectedRoute requiresTenant><AppRecharge /></ProtectedRoute>} />

      {/* Subscriptions */}
      <Route path="/products" element={<ProtectedRoute><ProductsPage /></ProtectedRoute>} />
      <Route path="/products/code/:productCode" element={<ProtectedRoute><ProductDetailPage /></ProtectedRoute>} />
      <Route path="/products/:productId" element={<ProtectedRoute><ProductDetailPage /></ProtectedRoute>} />
      <Route path="/subscriptions" element={<ProtectedRoute><SubscriptionIndex /></ProtectedRoute>} />
      <Route path="/subscriptions/checkout" element={<ProtectedRoute><SubscriptionCheckout /></ProtectedRoute>} />

      {/* Orders */}
      <Route path="/orders" element={<ProtectedRoute><OrderList /></ProtectedRoute>} />
      <Route path="/order" element={<ProtectedRoute><OrderList /></ProtectedRoute>} />
      <Route path="/orders/:orderId/confirm" element={<ProtectedRoute><OrderConfirm /></ProtectedRoute>} />
      <Route path="/order/:orderId/confirm" element={<ProtectedRoute><OrderConfirm /></ProtectedRoute>} />
      <Route path="/orders/:orderId/success" element={<ProtectedRoute><OrderSuccess /></ProtectedRoute>} />
      <Route path="/order/:orderId/success" element={<ProtectedRoute><OrderSuccess /></ProtectedRoute>} />

      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}
