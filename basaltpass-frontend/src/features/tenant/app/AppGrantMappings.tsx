import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeftIcon, ArrowRightIcon, ArrowsRightLeftIcon, PlusIcon, TrashIcon, XMarkIcon } from '@heroicons/react/24/outline'
import TenantLayout from '@features/tenant/components/TenantLayout'
import { PAlert, PBadge, PButton, PCard, PPageHeader, PSkeleton } from '@ui'
import { uiConfirm } from '@contexts/DialogContext'
import { useI18n } from '@shared/i18n'
import {
  appGrantMappingApi,
  type AppGrantMapping,
  type AppGrantMappingInput,
  type AppGrantMappingOptions,
  type GrantEndpoint,
  type GrantSourceType,
  type GrantTargetType,
} from '@api/tenant/appGrantMapping'

const emptyOptions: AppGrantMappingOptions = { membership_roles: [], tenant_roles: [], tenant_permissions: [], app_roles: [], app_permissions: [] }

function emptyForm(): AppGrantMappingInput {
  return { source_type: 'membership_role', source_id: 0, source_code: '', target_type: 'app_role', target_id: 0, enabled: true }
}

function toLocalDateTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function toPayloadDate(value: string) {
  return value ? new Date(value).toISOString() : undefined
}

export default function AppGrantMappings() {
  const { t, locale } = useI18n()
  const { id: appId } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [mappings, setMappings] = useState<AppGrantMapping[]>([])
  const [options, setOptions] = useState<AppGrantMappingOptions>(emptyOptions)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [showEditor, setShowEditor] = useState(false)
  const [editing, setEditing] = useState<AppGrantMapping | null>(null)
  const [form, setForm] = useState<AppGrantMappingInput>(emptyForm())
  const [validFrom, setValidFrom] = useState('')
  const [validUntil, setValidUntil] = useState('')
  const [previewCount, setPreviewCount] = useState<number | null>(null)

  const load = async () => {
    if (!appId) return
    try {
      setLoading(true)
      setError('')
      const [mappingItems, mappingOptions] = await Promise.all([appGrantMappingApi.list(appId), appGrantMappingApi.options(appId)])
      setMappings(mappingItems)
      setOptions(mappingOptions)
    } catch (err: any) {
      setError(err?.response?.data?.error || t('tenantAppGrantMappings.errors.load'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [appId])

  const sourceOptions = useMemo(() => {
    if (form.source_type === 'membership_role') return options.membership_roles
    if (form.source_type === 'tenant_role') return options.tenant_roles
    return options.tenant_permissions
  }, [form.source_type, options])

  const targetOptions = useMemo(() => form.target_type === 'app_role' ? options.app_roles : options.app_permissions, [form.target_type, options])

  const selectedSource = form.source_type === 'membership_role' ? form.source_code : String(form.source_id || '')

  const setSourceType = (sourceType: GrantSourceType) => {
    setForm({ ...form, source_type: sourceType, source_id: 0, source_code: '' })
    setPreviewCount(null)
  }

  const setSource = (value: string) => {
    setForm({ ...form, source_id: form.source_type === 'membership_role' ? 0 : Number(value), source_code: form.source_type === 'membership_role' ? value : '' })
    setPreviewCount(null)
  }

  const openCreate = () => {
    setEditing(null)
    setForm(emptyForm())
    setValidFrom('')
    setValidUntil('')
    setPreviewCount(null)
    setShowEditor(true)
  }

  const openEdit = (mapping: AppGrantMapping) => {
    setEditing(mapping)
    setForm({
      source_type: mapping.source.type as GrantSourceType,
      source_id: mapping.source.type === 'membership_role' ? 0 : Number(mapping.source.id || 0),
      source_code: mapping.source.type === 'membership_role' ? mapping.source.code : '',
      target_type: mapping.target.type as GrantTargetType,
      target_id: Number(mapping.target.id || 0),
      enabled: mapping.enabled,
      valid_from: mapping.valid_from,
      valid_until: mapping.valid_until,
    })
    setValidFrom(toLocalDateTime(mapping.valid_from))
    setValidUntil(toLocalDateTime(mapping.valid_until))
    setPreviewCount(mapping.affected_user_count)
    setShowEditor(true)
  }

  const payload = (): AppGrantMappingInput => ({ ...form, valid_from: toPayloadDate(validFrom), valid_until: toPayloadDate(validUntil) })

  const isComplete = Boolean(selectedSource && form.target_id)

  const preview = async () => {
    if (!appId || !isComplete) return
    try {
      setPreviewCount(await appGrantMappingApi.preview(appId, payload()))
    } catch (err: any) {
      setError(err?.response?.data?.error || t('tenantAppGrantMappings.errors.preview'))
    }
  }

  const save = async () => {
    if (!appId || !isComplete) return
    if (validFrom && validUntil && new Date(validUntil) <= new Date(validFrom)) {
      setError(t('tenantAppGrantMappings.errors.validity'))
      return
    }
    try {
      setSaving(true)
      setError('')
      if (editing) await appGrantMappingApi.update(appId, editing.id, payload())
      else await appGrantMappingApi.create(appId, payload())
      setShowEditor(false)
      await load()
    } catch (err: any) {
      setError(err?.response?.data?.error || t('tenantAppGrantMappings.errors.save'))
    } finally {
      setSaving(false)
    }
  }

  const toggle = async (mapping: AppGrantMapping) => {
    if (!appId) return
    const input: AppGrantMappingInput = {
      source_type: mapping.source.type as GrantSourceType,
      source_id: mapping.source.type === 'membership_role' ? 0 : Number(mapping.source.id || 0),
      source_code: mapping.source.type === 'membership_role' ? mapping.source.code : '',
      target_type: mapping.target.type as GrantTargetType,
      target_id: Number(mapping.target.id || 0),
      enabled: !mapping.enabled,
      valid_from: mapping.valid_from,
      valid_until: mapping.valid_until,
    }
    try {
      await appGrantMappingApi.update(appId, mapping.id, input)
      await load()
    } catch (err: any) {
      setError(err?.response?.data?.error || t('tenantAppGrantMappings.errors.save'))
    }
  }

  const remove = async (mapping: AppGrantMapping) => {
    if (!appId || !await uiConfirm(t('tenantAppGrantMappings.confirm.delete', { source: mapping.source.name, target: mapping.target.name }))) return
    try {
      await appGrantMappingApi.remove(appId, mapping.id)
      await load()
    } catch (err: any) {
      setError(err?.response?.data?.error || t('tenantAppGrantMappings.errors.delete'))
    }
  }

  return <TenantLayout title={t('tenantAppGrantMappings.title')}>
    <div className="space-y-6">
      <PPageHeader title={t('tenantAppGrantMappings.title')} description={t('tenantAppGrantMappings.description')}
        actions={<div className="flex gap-2"><PButton variant="secondary" onClick={() => navigate(`/tenant/apps/${appId}`)} leftIcon={<ArrowLeftIcon className="h-4 w-4" />}>{t('tenantAppGrantMappings.back')}</PButton><PButton onClick={openCreate} leftIcon={<PlusIcon className="h-4 w-4" />}>{t('tenantAppGrantMappings.actions.create')}</PButton></div>} />
      {error && <PAlert variant="error" message={error} />}
      <PAlert variant="info" message={t('tenantAppGrantMappings.dynamicNotice')} />
      {loading ? <PSkeleton.List items={3} showAvatar={false} /> : mappings.length === 0 ?
        <PCard className="p-10 text-center text-gray-500">{t('tenantAppGrantMappings.empty')}</PCard> :
        <div className="space-y-3">{mappings.map(mapping => <PCard key={mapping.id} className="p-5">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <div className="min-w-0 rounded-lg border bg-gray-50 px-4 py-3"><div className="text-xs text-gray-500">{t(`tenantAppGrantMappings.sourceTypes.${mapping.source.type}`)}</div><div className="truncate font-medium">{mapping.source.name}</div><code className="text-xs text-gray-500">{mapping.source.code}</code></div>
              <ArrowRightIcon className="h-5 w-5 shrink-0 text-indigo-500" />
              <div className="min-w-0 rounded-lg border border-indigo-100 bg-indigo-50 px-4 py-3"><div className="text-xs text-indigo-600">{t(`tenantAppGrantMappings.targetTypes.${mapping.target.type}`)}</div><div className="truncate font-medium">{mapping.target.name}</div><code className="text-xs text-gray-500">{mapping.target.code}</code></div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <PBadge variant={mapping.enabled ? 'success' : 'default'}>{mapping.enabled ? t('tenantAppGrantMappings.status.enabled') : t('tenantAppGrantMappings.status.disabled')}</PBadge>
              <PBadge variant="info">{t('tenantAppGrantMappings.affected', { count: mapping.affected_user_count })}</PBadge>
              <PButton variant="secondary" size="sm" onClick={() => toggle(mapping)}>{mapping.enabled ? t('tenantAppGrantMappings.actions.disable') : t('tenantAppGrantMappings.actions.enable')}</PButton>
              <PButton variant="secondary" size="sm" onClick={() => openEdit(mapping)}>{t('tenantAppGrantMappings.actions.edit')}</PButton>
              <PButton variant="danger" size="sm" onClick={() => remove(mapping)} leftIcon={<TrashIcon className="h-4 w-4" />}>{t('tenantAppGrantMappings.actions.delete')}</PButton>
            </div>
          </div>
          {(mapping.valid_from || mapping.valid_until) && <div className="mt-3 text-xs text-gray-500">{t('tenantAppGrantMappings.validity', { from: mapping.valid_from ? new Date(mapping.valid_from).toLocaleString(locale) : '—', until: mapping.valid_until ? new Date(mapping.valid_until).toLocaleString(locale) : '—' })}</div>}
        </PCard>)}</div>}
    </div>

    {showEditor && <div className="fixed inset-0 z-50 !m-0 flex items-center justify-center bg-gray-900/50 p-4">
      <div className="w-full max-w-2xl rounded-2xl bg-white p-6 shadow-xl">
        <div className="flex items-center justify-between"><h3 className="text-lg font-semibold">{editing ? t('tenantAppGrantMappings.editor.editTitle') : t('tenantAppGrantMappings.editor.createTitle')}</h3><button onClick={() => setShowEditor(false)}><XMarkIcon className="h-6 w-6" /></button></div>
        <div className="mt-5 grid gap-4 md:grid-cols-2">
          <label className="text-sm">{t('tenantAppGrantMappings.editor.sourceType')}<select className="mt-1 w-full rounded-lg border p-2" value={form.source_type} onChange={e => setSourceType(e.target.value as GrantSourceType)}><option value="membership_role">{t('tenantAppGrantMappings.sourceTypes.membership_role')}</option><option value="tenant_role">{t('tenantAppGrantMappings.sourceTypes.tenant_role')}</option><option value="tenant_permission">{t('tenantAppGrantMappings.sourceTypes.tenant_permission')}</option></select></label>
          <label className="text-sm">{t('tenantAppGrantMappings.editor.source')}<select className="mt-1 w-full rounded-lg border p-2" value={selectedSource} onChange={e => setSource(e.target.value)}><option value="">{t('tenantAppGrantMappings.editor.select')}</option>{sourceOptions.map((item: GrantEndpoint) => <option key={`${item.type}-${item.id || item.code}`} value={form.source_type === 'membership_role' ? item.code : item.id}>{item.name} ({item.code})</option>)}</select></label>
          <label className="text-sm">{t('tenantAppGrantMappings.editor.targetType')}<select className="mt-1 w-full rounded-lg border p-2" value={form.target_type} onChange={e => { setForm({ ...form, target_type: e.target.value as GrantTargetType, target_id: 0 }); setPreviewCount(null) }}><option value="app_role">{t('tenantAppGrantMappings.targetTypes.app_role')}</option><option value="app_permission">{t('tenantAppGrantMappings.targetTypes.app_permission')}</option></select></label>
          <label className="text-sm">{t('tenantAppGrantMappings.editor.target')}<select className="mt-1 w-full rounded-lg border p-2" value={form.target_id || ''} onChange={e => { setForm({ ...form, target_id: Number(e.target.value) }); setPreviewCount(null) }}><option value="">{t('tenantAppGrantMappings.editor.select')}</option>{targetOptions.map(item => <option key={`${item.type}-${item.id}`} value={item.id}>{item.name} ({item.code})</option>)}</select></label>
          <label className="text-sm">{t('tenantAppGrantMappings.editor.validFrom')}<input className="mt-1 w-full rounded-lg border p-2" type="datetime-local" value={validFrom} onChange={e => setValidFrom(e.target.value)} /></label>
          <label className="text-sm">{t('tenantAppGrantMappings.editor.validUntil')}<input className="mt-1 w-full rounded-lg border p-2" type="datetime-local" value={validUntil} onChange={e => setValidUntil(e.target.value)} /></label>
        </div>
        <label className="mt-4 flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} />{t('tenantAppGrantMappings.editor.enabled')}</label>
        {previewCount !== null && <div className="mt-4 rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800">{t('tenantAppGrantMappings.editor.previewResult', { count: previewCount })}</div>}
        <div className="mt-6 flex justify-end gap-2"><PButton variant="secondary" onClick={() => setShowEditor(false)}>{t('tenantAppGrantMappings.actions.cancel')}</PButton><PButton variant="secondary" disabled={!isComplete} onClick={preview} leftIcon={<ArrowsRightLeftIcon className="h-4 w-4" />}>{t('tenantAppGrantMappings.actions.preview')}</PButton><PButton disabled={!isComplete || saving} loading={saving} onClick={save}>{t('tenantAppGrantMappings.actions.save')}</PButton></div>
      </div>
    </div>}
  </TenantLayout>
}
