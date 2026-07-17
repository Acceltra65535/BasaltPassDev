import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ArrowLeftIcon,
  ArrowPathIcon,
  CheckCircleIcon,
  ClockIcon,
  CodeBracketSquareIcon,
  DocumentCheckIcon,
  ExclamationTriangleIcon,
  ShieldCheckIcon,
  XCircleIcon,
} from '@heroicons/react/24/outline'
import TenantLayout from '@features/tenant/components/TenantLayout'
import { tenantAppApi, type TenantApp } from '@api/tenant/tenantApp'
import {
  tenantRBACManifestApi,
  type RBACManifestDiff,
  type RBACManifestRecord,
  type RBACManifestStatus,
  type RBACRevisionRecord,
} from '@api/tenant/rbacManifest'
import { uiConfirm } from '@contexts/DialogContext'
import { PAlert, PBadge, PButton, PCard, PPageHeader, PSkeleton } from '@ui'
import { useI18n } from '@shared/i18n'

const emptyDiff: RBACManifestDiff = {
  has_changes: false,
  permissions_added: [],
  permissions_updated: [],
  permissions_removed: [],
  roles_added: [],
  roles_updated: [],
  roles_removed: [],
  role_permissions_added: [],
  role_permissions_removed: [],
  assigned_roles_affected: [],
  removal_assignment_blocks: [],
}

function statusVariant(status: RBACManifestStatus) {
  if (status === 'approved') return 'success'
  if (status === 'pending') return 'warning'
  if (status === 'rejected') return 'error'
  return 'default'
}

function DiffGroup({ title, values, tone = 'default' }: { title: string; values: string[]; tone?: 'default' | 'add' | 'remove' | 'warning' }) {
  if (!values.length) return null
  const styles = {
    default: 'border-gray-200 bg-gray-50 text-gray-700',
    add: 'border-green-200 bg-green-50 text-green-800',
    remove: 'border-red-200 bg-red-50 text-red-800',
    warning: 'border-yellow-200 bg-yellow-50 text-yellow-800',
  }
  return (
    <div className={`rounded-lg border p-3 ${styles[tone]}`}>
      <div className="mb-2 text-xs font-semibold uppercase tracking-wide">{title} · {values.length}</div>
      <div className="flex flex-wrap gap-2">
        {values.map((value) => <code key={value} className="rounded bg-white/80 px-2 py-1 text-xs">{value}</code>)}
      </div>
    </div>
  )
}

function ManifestDiff({ diff = emptyDiff, labels }: { diff?: RBACManifestDiff; labels: Record<string, string> }) {
  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <DiffGroup title={labels.permissionsAdded} values={diff.permissions_added || []} tone="add" />
      <DiffGroup title={labels.permissionsUpdated} values={diff.permissions_updated || []} />
      <DiffGroup title={labels.permissionsRemoved} values={diff.permissions_removed || []} tone="remove" />
      <DiffGroup title={labels.rolesAdded} values={diff.roles_added || []} tone="add" />
      <DiffGroup title={labels.rolesUpdated} values={diff.roles_updated || []} />
      <DiffGroup title={labels.rolesRemoved} values={diff.roles_removed || []} tone="remove" />
      <DiffGroup title={labels.rolePermissionsAdded} values={diff.role_permissions_added || []} tone="add" />
      <DiffGroup title={labels.rolePermissionsRemoved} values={diff.role_permissions_removed || []} tone="remove" />
      <DiffGroup title={labels.assignedRolesAffected} values={diff.assigned_roles_affected || []} tone="warning" />
      <DiffGroup title={labels.removalBlocks} values={diff.removal_assignment_blocks || []} tone="warning" />
    </div>
  )
}

export default function RBACManifests() {
  const { id } = useParams<{ id: string }>()
  const { t, locale } = useI18n()
  const [app, setApp] = useState<TenantApp | null>(null)
  const [manifests, setManifests] = useState<RBACManifestRecord[]>([])
  const [revisions, setRevisions] = useState<RBACRevisionRecord[]>([])
  const [expanded, setExpanded] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const labels = useMemo(() => ({
    permissionsAdded: t('tenantRBACManifests.diff.permissionsAdded'),
    permissionsUpdated: t('tenantRBACManifests.diff.permissionsUpdated'),
    permissionsRemoved: t('tenantRBACManifests.diff.permissionsRemoved'),
    rolesAdded: t('tenantRBACManifests.diff.rolesAdded'),
    rolesUpdated: t('tenantRBACManifests.diff.rolesUpdated'),
    rolesRemoved: t('tenantRBACManifests.diff.rolesRemoved'),
    rolePermissionsAdded: t('tenantRBACManifests.diff.rolePermissionsAdded'),
    rolePermissionsRemoved: t('tenantRBACManifests.diff.rolePermissionsRemoved'),
    assignedRolesAffected: t('tenantRBACManifests.diff.assignedRolesAffected'),
    removalBlocks: t('tenantRBACManifests.diff.removalBlocks'),
  }), [t])

  const load = useCallback(async () => {
    if (!id) return
    setLoading(true)
    setError('')
    try {
      const [appResponse, manifestResponse, revisionResponse] = await Promise.all([
        tenantAppApi.getTenantApp(id),
        tenantRBACManifestApi.list(id),
        tenantRBACManifestApi.listRevisions(id),
      ])
      setApp(appResponse.data)
      setManifests(manifestResponse)
      setRevisions(revisionResponse)
      setExpanded((current) => current ?? manifestResponse.find((item) => item.status === 'pending')?.id ?? manifestResponse[0]?.id ?? null)
    } catch (err: any) {
      setError(err.response?.data?.error || t('tenantRBACManifests.errors.load'))
    } finally {
      setLoading(false)
    }
  }, [id, t])

  useEffect(() => { void load() }, [load])

  const approve = async (manifest: RBACManifestRecord) => {
    if (!id) return
    if (manifest.diff.removal_assignment_blocks?.length) {
      setError(t('tenantRBACManifests.errors.blocked'))
      return
    }
    if (!await uiConfirm(t('tenantRBACManifests.confirm.approve', { revision: manifest.source_revision }))) return
    setBusy(`approve-${manifest.id}`)
    setError('')
    setSuccess('')
    try {
      await tenantRBACManifestApi.approve(id, manifest.id)
      setSuccess(t('tenantRBACManifests.success.approved', { revision: manifest.source_revision }))
      await load()
    } catch (err: any) {
      setError(err.response?.data?.error || t('tenantRBACManifests.errors.approve'))
    } finally {
      setBusy(null)
    }
  }

  const reject = async (manifest: RBACManifestRecord) => {
    if (!id || !await uiConfirm(t('tenantRBACManifests.confirm.reject', { revision: manifest.source_revision }))) return
    setBusy(`reject-${manifest.id}`)
    setError('')
    try {
      await tenantRBACManifestApi.reject(id, manifest.id)
      setSuccess(t('tenantRBACManifests.success.rejected', { revision: manifest.source_revision }))
      await load()
    } catch (err: any) {
      setError(err.response?.data?.error || t('tenantRBACManifests.errors.reject'))
    } finally {
      setBusy(null)
    }
  }

  const rollback = async (revision: RBACRevisionRecord) => {
    if (!id || revision.is_active || !await uiConfirm(t('tenantRBACManifests.confirm.rollback', { revision: revision.revision }))) return
    setBusy(`rollback-${revision.id}`)
    setError('')
    try {
      await tenantRBACManifestApi.rollback(id, revision.id)
      setSuccess(t('tenantRBACManifests.success.rolledBack', { revision: revision.revision }))
      await load()
    } catch (err: any) {
      setError(err.response?.data?.error || t('tenantRBACManifests.errors.rollback'))
    } finally {
      setBusy(null)
    }
  }

  if (loading) {
    return <TenantLayout title={t('tenantRBACManifests.title')}><div className="space-y-4"><PSkeleton className="h-24" /><PSkeleton className="h-64" /></div></TenantLayout>
  }

  return (
    <TenantLayout title={t('tenantRBACManifests.title')}>
      <div className="space-y-6">
        <div className="flex flex-col justify-between gap-4 md:flex-row md:items-start">
          <div>
            <Link to={`/tenant/apps/${id}`} className="mb-3 inline-flex items-center text-sm text-gray-600 hover:text-indigo-600">
              <ArrowLeftIcon className="mr-1 h-4 w-4" />{t('tenantRBACManifests.back')}
            </Link>
            <PPageHeader title={t('tenantRBACManifests.title')} description={t('tenantRBACManifests.description', { app: app?.name || '' })} />
          </div>
          <PButton variant="secondary" onClick={() => void load()} leftIcon={<ArrowPathIcon className="h-4 w-4" />}>
            {t('tenantRBACManifests.refresh')}
          </PButton>
        </div>

        {error && <PAlert variant="error" message={error} dismissible onDismiss={() => setError('')} />}
        {success && <PAlert variant="success" message={success} dismissible onDismiss={() => setSuccess('')} />}

        <PAlert variant="info" title={t('tenantRBACManifests.integration.title')}>
          <div className="space-y-2">
            <p>{t('tenantRBACManifests.integration.description')}</p>
            <div className="flex flex-wrap gap-2">
              <code className="rounded bg-blue-100 px-2 py-1">POST /api/v1/s2s/rbac/manifests</code>
              <code className="rounded bg-blue-100 px-2 py-1">scope: s2s.rbac.manifest.submit</code>
            </div>
          </div>
        </PAlert>

        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="flex items-center text-lg font-semibold text-gray-900"><DocumentCheckIcon className="mr-2 h-5 w-5 text-indigo-600" />{t('tenantRBACManifests.manifests.title')}</h2>
            <PBadge variant="info">{manifests.length}</PBadge>
          </div>
          {!manifests.length ? (
            <PCard variant="bordered" className="py-12 text-center">
              <CodeBracketSquareIcon className="mx-auto h-10 w-10 text-gray-400" />
              <p className="mt-3 font-medium text-gray-900">{t('tenantRBACManifests.manifests.emptyTitle')}</p>
              <p className="mt-1 text-sm text-gray-500">{t('tenantRBACManifests.manifests.emptyDescription')}</p>
            </PCard>
          ) : manifests.map((manifest) => {
            const open = expanded === manifest.id
            const blocked = Boolean(manifest.diff.removal_assignment_blocks?.length)
            return (
              <PCard key={manifest.id} variant="bordered" padding="none" className="overflow-hidden">
                <button type="button" className="flex w-full flex-col gap-3 p-5 text-left md:flex-row md:items-center md:justify-between" onClick={() => setExpanded(open ? null : manifest.id)}>
                  <div className="flex items-start gap-3">
                    {manifest.status === 'approved' ? <CheckCircleIcon className="h-6 w-6 text-green-600" /> : manifest.status === 'rejected' ? <XCircleIcon className="h-6 w-6 text-red-600" /> : <ClockIcon className="h-6 w-6 text-yellow-600" />}
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-semibold text-gray-900">{t('tenantRBACManifests.manifests.revision', { revision: manifest.source_revision })}</span>
                        <PBadge variant={statusVariant(manifest.status)}>{t(`tenantRBACManifests.status.${manifest.status}`)}</PBadge>
                        {blocked && <PBadge variant="error">{t('tenantRBACManifests.manifests.blocked')}</PBadge>}
                      </div>
                      <div className="mt-1 text-xs text-gray-500">{new Date(manifest.submitted_at).toLocaleString(locale)} · {manifest.source_client_id} · {manifest.digest.slice(0, 12)}</div>
                    </div>
                  </div>
                  <div className="text-sm text-gray-600">
                    +{manifest.diff.permissions_added.length + manifest.diff.roles_added.length + manifest.diff.role_permissions_added.length} / −{manifest.diff.permissions_removed.length + manifest.diff.roles_removed.length + manifest.diff.role_permissions_removed.length}
                  </div>
                </button>
                {open && (
                  <div className="border-t border-gray-200 bg-white p-5">
                    {!manifest.diff.has_changes && <PAlert variant="info" message={t('tenantRBACManifests.manifests.noChanges')} className="mb-4" />}
                    {blocked && <PAlert variant="warning" title={t('tenantRBACManifests.manifests.blockedTitle')} message={t('tenantRBACManifests.manifests.blockedDescription')} className="mb-4" />}
                    <ManifestDiff diff={manifest.diff} labels={labels} />
                    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4">
                      <div className="text-xs text-gray-500">{manifest.manifest.permissions.length} {t('tenantRBACManifests.manifests.permissions')} · {manifest.manifest.roles.length} {t('tenantRBACManifests.manifests.roles')}</div>
                      {manifest.status === 'pending' && (
                        <div className="flex gap-2">
                          <PButton variant="secondary" loading={busy === `reject-${manifest.id}`} disabled={Boolean(busy)} onClick={() => void reject(manifest)}>{t('tenantRBACManifests.actions.reject')}</PButton>
                          <PButton loading={busy === `approve-${manifest.id}`} disabled={Boolean(busy) || blocked} leftIcon={<ShieldCheckIcon className="h-4 w-4" />} onClick={() => void approve(manifest)}>{t('tenantRBACManifests.actions.approve')}</PButton>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </PCard>
            )
          })}
        </section>

        <section className="space-y-3">
          <h2 className="flex items-center text-lg font-semibold text-gray-900"><ArrowPathIcon className="mr-2 h-5 w-5 text-indigo-600" />{t('tenantRBACManifests.revisions.title')}</h2>
          <PCard variant="bordered" padding="none">
            {!revisions.length ? <div className="p-8 text-center text-sm text-gray-500">{t('tenantRBACManifests.revisions.empty')}</div> : (
              <div className="divide-y divide-gray-200">
                {revisions.map((revision) => (
                  <div key={revision.id} className="flex flex-col gap-3 p-4 md:flex-row md:items-center md:justify-between">
                    <div className="flex items-center gap-3">
                      {revision.is_active ? <ShieldCheckIcon className="h-5 w-5 text-green-600" /> : <ClockIcon className="h-5 w-5 text-gray-400" />}
                      <div>
                        <div className="flex items-center gap-2"><span className="font-medium text-gray-900">#{revision.revision}</span><PBadge variant={revision.is_active ? 'success' : 'default'}>{revision.is_active ? t('tenantRBACManifests.revisions.active') : t(`tenantRBACManifests.revisions.${revision.action}`)}</PBadge></div>
                        <div className="mt-1 text-xs text-gray-500">{new Date(revision.created_at).toLocaleString(locale)} · {revision.digest.slice(0, 12)}</div>
                      </div>
                    </div>
                    {!revision.is_active && <PButton size="sm" variant="secondary" loading={busy === `rollback-${revision.id}`} disabled={Boolean(busy)} leftIcon={<ArrowPathIcon className="h-4 w-4" />} onClick={() => void rollback(revision)}>{t('tenantRBACManifests.actions.rollback')}</PButton>}
                  </div>
                ))}
              </div>
            )}
          </PCard>
          <PAlert variant="warning" title={t('tenantRBACManifests.revisions.safetyTitle')}>
            <span className="inline-flex items-start"><ExclamationTriangleIcon className="mr-2 mt-0.5 h-4 w-4 shrink-0" />{t('tenantRBACManifests.revisions.safetyDescription')}</span>
          </PAlert>
        </section>
      </div>
    </TenantLayout>
  )
}
