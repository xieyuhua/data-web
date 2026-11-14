// 组织结构相关常量与工具函数：
// 组织层级固定为「集团 → 大区 → 区域 → 门店 → 员工」，用于下钻、筛选与数据权限展示
export const ORG_TYPES = [
  { value: 'company', label: '集团' },
  { value: 'big_region', label: '大区' },
  { value: 'region', label: '区域' },
  { value: 'store', label: '门店' },
]

// 组织类型值 => 中文名称映射（额外补充“员工”类型）
const ORG_TYPE_MAP = {
  ...Object.fromEntries(ORG_TYPES.map((item) => [item.value, item.label])),
  employee: '员工',
}

// 数据权限范围文案映射
export const DATA_SCOPE_LABELS = {
  all: '全部组织',
  own_org_tree: '本组织及下级',
  own_org: '仅本组织',
}

// 组织类型转中文，未知类型原样返回
export function orgTypeLabel(type) {
  return ORG_TYPE_MAP[type] || type || '组织'
}

// 数据权限范围转中文
export function dataScopeLabel(scope) {
  return DATA_SCOPE_LABELS[scope] || scope || '未配置'
}

// 把嵌套组织树拍平成一维数组，并记录层级 level 与父节点名称，供表格展示
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

// 组织树 => 树选择器选项；已停用的组织不可选
export function toTreeOptions(tree) {
  return (tree || []).map((item) => ({
    value: item.id,
    label: item.name,
    type: item.type,
    disabled: item.status === 'disabled',
    children: toTreeOptions(item.children),
  }))
}

// 在组织树中查找目标节点的完整链路（祖先 -> 目标），用于面包屑和层级判断
export function findOrgChain(tree, targetId) {
  if (!targetId) return []
  for (const item of tree || []) {
    if (item.id === targetId) return [item]
    const childChain = findOrgChain(item.children, targetId)
    if (childChain.length) return [item, ...childChain]
  }
  return []
}

// 按 ID 查找组织节点本身（链路最后一个节点），找不到返回 null
export function findOrgById(tree, targetId) {
  return findOrgChain(tree, targetId).at(-1) || null
}
