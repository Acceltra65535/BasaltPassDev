import client from '../client'

export type GrantSourceType = 'membership_role' | 'tenant_role' | 'tenant_permission'
export type GrantTargetType = 'app_role' | 'app_permission'

export interface GrantEndpoint {
  type: GrantSourceType | GrantTargetType
  id?: number
  code: string
  name: string
}

export interface AppGrantMapping {
  id: number
  tenant_id: number
  app_id: number
  source: GrantEndpoint
  target: GrantEndpoint
  enabled: boolean
  valid_from?: string
  valid_until?: string
  affected_user_count: number
  created_by: number
  updated_by: number
  created_at: string
  updated_at: string
}

export interface AppGrantMappingInput {
  source_type: GrantSourceType
  source_id: number
  source_code: string
  target_type: GrantTargetType
  target_id: number
  enabled: boolean
  valid_from?: string
  valid_until?: string
}

export interface AppGrantMappingOptions {
  membership_roles: GrantEndpoint[]
  tenant_roles: GrantEndpoint[]
  tenant_permissions: GrantEndpoint[]
  app_roles: GrantEndpoint[]
  app_permissions: GrantEndpoint[]
}

export interface GrantSource {
  type: 'explicit' | 'tenant_mapping'
  assignment_id?: number
  mapping_id?: number
  source_type?: GrantSourceType
  source_id?: number
  source_code?: string
  via_role_code?: string
}

export interface EffectiveRole {
  id: number
  code: string
  name: string
  description: string
  app_id: number
  permissions: Array<{ id: number; code: string; name: string }>
  sources: GrantSource[]
}

export interface EffectivePermission {
  id: number
  code: string
  name: string
  description: string
  category: string
  app_id: number
  sources: GrantSource[]
}

export interface EffectiveGrants {
  eligible: boolean
  denial_reason?: string
  roles: EffectiveRole[]
  permissions: EffectivePermission[]
}

export const appGrantMappingApi = {
  async list(appId: string) {
    const response = await client.get(`/api/v1/tenant/apps/${appId}/rbac/mappings`)
    return (response.data?.data?.mappings || []) as AppGrantMapping[]
  },

  async options(appId: string) {
    const response = await client.get(`/api/v1/tenant/apps/${appId}/rbac/mappings/options`)
    return response.data.data as AppGrantMappingOptions
  },

  async preview(appId: string, input: AppGrantMappingInput) {
    const response = await client.post(`/api/v1/tenant/apps/${appId}/rbac/mappings/preview`, input)
    return Number(response.data?.data?.affected_user_count || 0)
  },

  async create(appId: string, input: AppGrantMappingInput) {
    const response = await client.post(`/api/v1/tenant/apps/${appId}/rbac/mappings`, input)
    return response.data.data as AppGrantMapping
  },

  async update(appId: string, mappingId: number, input: AppGrantMappingInput) {
    const response = await client.put(`/api/v1/tenant/apps/${appId}/rbac/mappings/${mappingId}`, input)
    return response.data.data as AppGrantMapping
  },

  async remove(appId: string, mappingId: number) {
    await client.delete(`/api/v1/tenant/apps/${appId}/rbac/mappings/${mappingId}`)
  },

  async effectiveGrants(appId: string, userId: string) {
    const response = await client.get(`/api/v1/tenant/apps/${appId}/users/${userId}/effective-grants`)
    return response.data.data as EffectiveGrants
  },
}
