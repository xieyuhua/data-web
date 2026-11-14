export const ORG_TYPES = [
  { value: 'company', label: '集团' },
  { value: 'big_region', label: '大区' },
  { value: 'region', label: '区域' },
  { value: 'store', label: '门店' },
]

const ORG_TYPE_MAP = {
  ...Object.fromEntries(ORG_TYPES.map((item) => [item.value, item.label])),
  employee: '员工',
}

export const DATA_SCOPE_LABELS = {
  all: '全部组织',
  own_org_tree: '本组织及下级',
  own_org: '仅本组织',
}

export function orgTypeLabel(type) {
  return ORG_TYPE_MAP[type] || type || '组织'
}

export function dataScopeLabel(scope) {
  return DATA_SCOPE_LABELS[scope] || scope || '未配置'
}

export function flattenOrgs(tree, level = 1, parentName = '') {
  const result = []
  for (const item of tree || []) {
    result.push({ ...item, level, parent_name: parentName })
    if (item.children?.length) {
      result.push(...flattenOrgs(item.children, level + 1, item.name))
    }
  }
  return result
}

export function toTreeOptions(tree) {
  return (tree || []).map((item) => ({
    value: item.id,
    label: item.name,
    type: item.type,
    disabled: item.status === 'disabled',
    children: toTreeOptions(item.children),
  }))
}

export function findOrgChain(tree, targetId) {
  if (!targetId) return []
  for (const item of tree || []) {
    if (item.id === targetId) return [item]
    const childChain = findOrgChain(item.children, targetId)
    if (childChain.length) return [item, ...childChain]
  }
  return []
}

export function findOrgById(tree, targetId) {
  return findOrgChain(tree, targetId).at(-1) || null
}
