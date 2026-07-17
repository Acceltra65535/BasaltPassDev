import client from '../client'

export type RBACManifestStatus = 'pending' | 'approved' | 'rejected' | 'superseded'

export interface RBACManifestDiff {
  has_changes: boolean
  permissions_added: string[]
  permissions_updated: string[]
  permissions_removed: string[]
  roles_added: string[]
  roles_updated: string[]
  roles_removed: string[]
  role_permissions_added: string[]
  role_permissions_removed: string[]
  assigned_roles_affected: string[]
  removal_assignment_blocks: string[]
}

export interface RBACManifestPermission {
  permission_key: string
  display_name: string
  resource: string
  action: string
  scope: string
  description: string
  status: string
}

export interface RBACManifestRole {
  role_key: string
  display_name: string
  description: string
  assignable: boolean
  priority: number
  status: string
}

export interface RBACManifestPayload {
  schema_version: string
  type: 'basalt_rbac_bundle'
  revision: number
  permissions: RBACManifestPermission[]
  roles: RBACManifestRole[]
  role_permissions: Array<{ role_key: string; permission_key: string; effect: 'allow' }>
}

export interface RBACManifestRecord {
  id: number
  tenant_id: number
  app_id: number
  source_client_id: string
  schema_version: string
  source_revision: number
  digest: string
  base_digest: string
  status: RBACManifestStatus
  diff: RBACManifestDiff
  manifest: RBACManifestPayload
  submitted_at: string
  reviewed_at?: string
  reviewed_by?: number
  review_note?: string
  active_revision_id?: number
}

export interface RBACRevisionRecord {
  id: number
  tenant_id: number
  app_id: number
  revision: number
  digest: string
  manifest_id?: number
  action: 'baseline' | 'manifest' | 'rollback'
  target_revision_id?: number
  is_active: boolean
  created_by: number
  created_at: string
}

export const tenantRBACManifestApi = {
  async list(appId: string) {
    const response = await client.get(`/api/v1/tenant/apps/${appId}/rbac/manifests`)
    return response.data.data.manifests as RBACManifestRecord[]
  },

  async approve(appId: string, manifestId: number) {
    const response = await client.post(`/api/v1/tenant/apps/${appId}/rbac/manifests/${manifestId}/approve`)
    return response.data.data as RBACManifestRecord
  },

  async reject(appId: string, manifestId: number, note = '') {
    const response = await client.post(`/api/v1/tenant/apps/${appId}/rbac/manifests/${manifestId}/reject`, { note })
    return response.data.data as RBACManifestRecord
  },

  async listRevisions(appId: string) {
    const response = await client.get(`/api/v1/tenant/apps/${appId}/rbac/revisions`)
    return response.data.data.revisions as RBACRevisionRecord[]
  },

  async rollback(appId: string, revisionId: number) {
    const response = await client.post(`/api/v1/tenant/apps/${appId}/rbac/revisions/${revisionId}/rollback`)
    return response.data.data as RBACRevisionRecord
  },
}
