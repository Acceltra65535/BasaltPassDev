const cjkPattern = /[\u3400-\u9fff]/

const directMap: Record<string, string> = {
  用户管理: 'User Management',
  用户权限: 'User Permissions',
  内容管理: 'Content Management',
  系统设置: 'System Settings',
  数据分析: 'Data Analytics',
  财务管理: 'Finance Management',
  权限管理: 'Permission Management',
  审计日志: 'Audit Logs',
  通知管理: 'Notification Management',
  管理员: 'Administrator',
  所有者: 'Owner',
  成员: 'Member',
  访客: 'Viewer',
}

const actionLabels: Record<string, string> = {
  add: 'Create',
  assign: 'Assign',
  audit: 'View',
  create: 'Create',
  delete: 'Delete',
  edit: 'Edit',
  export: 'Export',
  import: 'Import',
  invite: 'Invite',
  list: 'View',
  manage: 'Manage',
  read: 'View',
  remove: 'Delete',
  update: 'Edit',
  view: 'View',
  write: 'Edit',
}

const roleLabels: Record<string, string> = {
  admin: 'Administrator',
  administrator: 'Administrator',
  member: 'Member',
  owner: 'Owner',
  user: 'Member',
  viewer: 'Viewer',
}

const titleCase = (value: string) =>
  value
    .replace(/[_-]+/g, ' ')
    .split(' ')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')

const inferNameFromCode = (code?: string) => {
  if (!code) return ''
  const parts = code.split('.').filter(Boolean)
  const last = parts.at(-1) || ''
  const previous = parts.at(-2) || ''

  if (roleLabels[last]) return roleLabels[last]

  const action = actionLabels[last] || titleCase(last)
  const resource = previous ? titleCase(previous) : ''
  return resource ? `${action} ${resource}` : action
}

const inferDescriptionFromCode = (code?: string) => {
  const name = inferNameFromCode(code)
  return name ? `Allows ${name.toLowerCase()}.` : ''
}

const inferCategoryFromCode = (code?: string) => {
  if (!code) return ''
  const parts = code.split('.').filter(Boolean)
  const category = parts.length > 2 ? parts.at(-2) : parts.at(0)
  return category ? titleCase(category) : ''
}

export const displayAccessName = (value?: string | null, code?: string) => {
  const text = value?.trim()
  if (!text) return inferNameFromCode(code) || '-'
  if (!cjkPattern.test(text)) return text
  return directMap[text] || inferNameFromCode(code) || text
}

export const displayAccessDescription = (value?: string | null, code?: string) => {
  const text = value?.trim()
  if (!text) return '-'
  if (!cjkPattern.test(text)) return text
  return directMap[text] || inferDescriptionFromCode(code) || text
}

export const displayAccessCategory = (value?: string | null, code?: string) => {
  const text = value?.trim()
  if (!text) return inferCategoryFromCode(code) || '-'
  if (!cjkPattern.test(text)) return text
  return directMap[text] || inferCategoryFromCode(code) || text
}
