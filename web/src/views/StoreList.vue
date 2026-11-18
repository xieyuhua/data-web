<template>
  <div class="page-card">
    <div class="page-header">
      <span class="page-title">门店列表</span>
      <div class="flex-gap">
        <el-input
          v-model="keyword"
          placeholder="门店编码 / 名称 / 地址"
          clearable
          class="filter-input"
          @keyup.enter="reload"
          @clear="reload"
        />
        <el-select v-model="bigRegionCode" clearable placeholder="全部大区" class="filter-select" @change="onBigChange" filterable>
          <el-option v-for="b in bigRegions" :key="b.code" :label="`${b.name} (${b.code})`" :value="b.code" />
        </el-select>
        <el-select v-model="regionCode" clearable filterable placeholder="全部区域" class="filter-select" @change="reload">
          <el-option
            v-for="r in regionOptions"
            :key="r.code"
            :label="`${r.name} (${r.code})`"
            :value="r.code"
          />
        </el-select>
        <el-select v-model="status" clearable placeholder="全部状态" style="width: 110px" @change="reload" filterable>
          <el-option label="营业中" value="active" />
          <el-option label="已关闭" value="closed" />
        </el-select>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增门店</el-button>
        <el-dropdown trigger="click" @command="onBulk">
          <el-button :icon="More">批量</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="template">下载导入模板</el-dropdown-item>
              <el-dropdown-item command="import">导入门店 Excel</el-dropdown-item>
              <el-dropdown-item command="report">导入核对报告</el-dropdown-item>
              <el-dropdown-item command="export" divided>导出门店列表</el-dropdown-item>
              <el-dropdown-item command="regions" divided>区域编码维护</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <div class="table-scroll">
      <el-table :data="list" v-loading="loading" border stripe size="small">
        <el-table-column prop="code" label="门店编码" width="120" fixed />
        <el-table-column prop="name" label="门店名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="大区" width="130">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.bigRegionName || row.bigRegionCode }}</el-tag>
            <span class="muted code">{{ row.bigRegionCode }}</span>
          </template>
        </el-table-column>
        <el-table-column label="区域" width="150">
          <template #default="{ row }">
            <el-tag size="small" effect="plain" type="info">{{ row.regionName || row.regionCode }}</el-tag>
            <span class="muted code">{{ row.regionCode }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="address" label="地址" min-width="200" show-overflow-tooltip />
        <el-table-column prop="contact" label="联系人" width="100" />
        <el-table-column prop="phone" label="电话" width="130" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'active' ? 'success' : 'info'">
              {{ row.status === 'active' ? '营业中' : '已关闭' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="开业日期" width="110">
          <template #default="{ row }">{{ row.openedAt ? row.openedAt.slice(0, 10) : '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-pagination
      class="pager"
      layout="total, prev, pager, next"
      :total="total"
      :page-size="pageSize"
      :current-page="page"
      @current-change="(p: number) => { page = p; load() }"
    />

    <!-- Create / edit store -->
    <el-dialog v-model="dialogVisible" :title="editing ? '编辑门店' : '新增门店'" :width="dialogWidth(560)" destroy-on-close>
      <el-form label-width="96px">
        <el-form-item label="门店编码" required>
          <el-input v-model="form.code" placeholder="如 SH001" :disabled="!!editing" />
          <div v-if="editing" class="muted" style="font-size: 12px">编码为唯一标识，创建后不可修改</div>
        </el-form-item>
        <el-form-item label="门店名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="大区" required>
          <el-select v-model="form.bigRegionId" class="full" @change="onFormBigChange" filterable>
            <el-option v-for="b in bigRegions" :key="b.id" :label="`${b.name} (${b.code})`" :value="b.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="区域" required>
          <el-select
            v-model="form.regionId"
            class="full"
            filterable
            :disabled="!form.bigRegionId"
            :placeholder="form.bigRegionId ? '搜索区域名称/编码/拼音' : '请先选择大区'"
          >
            <el-option
              v-for="r in formRegions"
              :key="r.id"
              :label="`${r.name} (${r.code})`"
              :value="r.id"
            >
              <span>{{ r.name }}</span>
              <span class="muted" style="float: right">{{ r.code }} · {{ r.pinyin }}</span>
            </el-option>
          </el-select>
          <div v-if="selectedRegion" class="muted" style="font-size: 12px; margin-top: 2px">
            区域编码 {{ selectedRegion.code }} · 拼音 {{ selectedRegion.pinyin }}（{{ selectedRegion.initials }}）
          </div>
        </el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.address" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="联系人" label-width="96px">
              <el-input v-model="form.contact" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="电话" label-width="60px">
              <el-input v-model="form.phone" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="状态" label-width="96px">
              <el-radio-group v-model="form.status">
                <el-radio-button value="active">营业中</el-radio-button>
                <el-radio-button value="closed">已关闭</el-radio-button>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="开业日期" label-width="80px">
              <el-date-picker v-model="form.openedAt" type="date" value-format="YYYY-MM-DD" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="备注">
          <el-input v-model="form.remarks" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- Region maintenance -->
    <el-dialog v-model="regionDialog" title="大区 / 区域编码维护" :width="dialogWidth(860)" top="5vh">
      <el-tabs v-model="regionTab">
        <el-tab-pane label="大区" name="big">
          <div class="flex-gap" style="margin-bottom: 10px">
            <el-input v-model="bigForm.code" placeholder="大区编码 如 HD" style="width: 150px" />
            <el-input v-model="bigForm.name" placeholder="大区名称 如 华东大区" style="width: 200px" />
            <el-button type="primary" @click="saveBig">{{ bigForm.id ? '保存' : '新增' }}</el-button>
            <el-button v-if="bigForm.id" @click="resetBigForm">取消编辑</el-button>
          </div>
          <div class="table-scroll">
            <el-table :data="bigRegions" size="small" border max-height="360">
              <el-table-column prop="code" label="编码" width="110" />
              <el-table-column prop="name" label="名称" min-width="140" />
              <el-table-column prop="pinyin" label="拼音" min-width="140" />
              <el-table-column prop="initials" label="首字母" width="100" />
              <el-table-column label="操作" width="120">
                <template #default="{ row }">
                  <el-button link type="primary" @click="editBig(row)">编辑</el-button>
                  <el-button link type="danger" @click="removeBig(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <el-tab-pane label="区域" name="region">
          <div class="flex-gap" style="margin-bottom: 10px">
            <el-select v-model="regionForm.bigRegionId" placeholder="所属大区" style="width: 160px" filterable>
              <el-option v-for="b in bigRegions" :key="b.id" :label="b.name" :value="b.id" />
            </el-select>
            <el-input v-model="regionForm.code" placeholder="区域编码 如 HD-SH" style="width: 150px" />
            <el-input v-model="regionForm.name" placeholder="区域名称 如 上海区域" style="width: 180px" />
            <el-button type="primary" @click="saveRegion">{{ regionForm.id ? '保存' : '新增' }}</el-button>
            <el-button v-if="regionForm.id" @click="resetRegionForm">取消编辑</el-button>
          </div>
          <el-input v-model="regionKeyword" placeholder="按名称/编码/拼音检索区域" clearable style="margin-bottom: 10px" @input="loadRegions" />
          <div class="table-scroll">
            <el-table :data="allRegions" size="small" border max-height="320">
              <el-table-column prop="code" label="编码" width="120" />
              <el-table-column prop="name" label="名称" min-width="140" />
              <el-table-column label="所属大区" width="130">
                <template #default="{ row }">{{ bigName(row.bigRegionId) }}</template>
              </el-table-column>
              <el-table-column prop="pinyin" label="拼音" min-width="140" />
              <el-table-column label="操作" width="120">
                <template #default="{ row }">
                  <el-button link type="primary" @click="editRegion(row)">编辑</el-button>
                  <el-button link type="danger" @click="removeRegion(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-dialog>

    <!-- Import result -->
    <el-dialog v-model="importDialog" title="导入结果" :width="dialogWidth(680)">
      <el-descriptions :column="4" border size="small" style="margin-bottom: 12px">
        <el-descriptions-item label="总行数">{{ importResult?.total }}</el-descriptions-item>
        <el-descriptions-item label="新增">{{ importResult?.created }}</el-descriptions-item>
        <el-descriptions-item label="更新">{{ importResult?.updated }}</el-descriptions-item>
        <el-descriptions-item label="失败">
          <span :style="{ color: importResult?.failed ? '#f56c6c' : '' }">{{ importResult?.failed }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <div class="table-scroll">
        <el-table :data="importResult?.issues || []" size="small" border max-height="300">
          <el-table-column prop="row" label="行号" width="70" />
          <el-table-column prop="key" label="门店编码" width="130" />
          <el-table-column label="级别" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.level === 'error' ? 'danger' : 'warning'">
                {{ row.level === 'error' ? '错误' : '提示' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="原因" min-width="240" />
        </el-table>
      </div>
      <template #footer>
        <el-button @click="importDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <input ref="fileInput" type="file" accept=".xlsx" style="display: none" @change="onFilePicked" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { More, Plus } from '@element-plus/icons-vue'
import {
  territoryApi,
  type BigRegion,
  type ImportResult,
  type Store,
  type TerritoryRegion,
} from '@/api'
import { dialogWidth } from '@/utils/responsive'

const list = ref<Store[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)

const keyword = ref('')
const bigRegionCode = ref('')
const regionCode = ref('')
const status = ref('')

const bigRegions = ref<BigRegion[]>([])
const allRegions = ref<TerritoryRegion[]>([])
const regionKeyword = ref('')

const dialogVisible = ref(false)
const editing = ref<Store | null>(null)
const saving = ref(false)
const form = reactive({
  code: '', name: '', bigRegionId: undefined as number | undefined,
  regionId: undefined as number | undefined,
  address: '', contact: '', phone: '', status: 'active', openedAt: '', remarks: '',
})

const formRegions = computed(() =>
  allRegions.value.filter((r) => r.bigRegionId === form.bigRegionId),
)
const selectedRegion = computed(() => allRegions.value.find((r) => r.id === form.regionId))

const regionOptions = computed(() =>
  bigRegionCode.value
    ? allRegions.value.filter((r) => bigCode(r.bigRegionId) === bigRegionCode.value)
    : allRegions.value,
)

function bigCode(bigId: number) {
  return bigRegions.value.find((b) => b.id === bigId)?.code || ''
}
function bigName(bigId: number) {
  return bigRegions.value.find((b) => b.id === bigId)?.name || `#${bigId}`
}

async function load() {
  loading.value = true
  try {
    const res = await territoryApi.stores({
      keyword: keyword.value,
      bigRegionCode: bigRegionCode.value,
      regionCode: regionCode.value,
      status: status.value,
      page: page.value,
      pageSize,
    })
    list.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  load()
}

function onBigChange() {
  regionCode.value = ''
  reload()
}

async function loadTerritory() {
  const [bigs, regions] = await Promise.all([
    territoryApi.bigRegions(),
    territoryApi.regions({ keyword: regionKeyword.value }),
  ])
  bigRegions.value = bigs
  allRegions.value = regions
}

function loadRegions() {
  loadTerritory()
}

// ---------- Store form ----------

function resetForm() {
  Object.assign(form, {
    code: '', name: '', bigRegionId: undefined, regionId: undefined,
    address: '', contact: '', phone: '', status: 'active', openedAt: '', remarks: '',
  })
}

function openCreate() {
  editing.value = null
  resetForm()
  if (bigRegions.value.length === 1) {
    form.bigRegionId = bigRegions.value[0].id
  }
  dialogVisible.value = true
}

function openEdit(row: Store) {
  editing.value = row
  Object.assign(form, {
    code: row.code, name: row.name,
    bigRegionId: row.bigRegionId, regionId: row.regionId,
    address: row.address, contact: row.contact, phone: row.phone,
    status: row.status, openedAt: row.openedAt ? row.openedAt.slice(0, 10) : '',
    remarks: row.remarks || '',
  })
  dialogVisible.value = true
}

function onFormBigChange() {
  form.regionId = undefined
}

async function save() {
  if (!form.code || !form.name) {
    ElMessage.warning('请填写门店编码与名称')
    return
  }
  if (!form.regionId) {
    ElMessage.warning('请选择所属区域')
    return
  }
  saving.value = true
  try {
    await territoryApi.saveStore({ ...form } as any, editing.value?.id)
    ElMessage.success('已保存')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function remove(row: Store) {
  await ElMessageBox.confirm(`确定删除门店「${row.name}（${row.code}）」吗？`, '删除确认', { type: 'warning' })
  await territoryApi.removeStore(row.id)
  ElMessage.success('已删除')
  await load()
}

// ---------- Region maintenance ----------

const regionDialog = ref(false)
const regionTab = ref('big')
const bigForm = reactive({ id: 0, code: '', name: '' })
const regionForm = reactive({ id: 0, code: '', name: '', bigRegionId: undefined as number | undefined })

function resetBigForm() {
  Object.assign(bigForm, { id: 0, code: '', name: '' })
}
function resetRegionForm() {
  Object.assign(regionForm, { id: 0, code: '', name: '', bigRegionId: undefined })
}

function editBig(row: BigRegion) {
  Object.assign(bigForm, { id: row.id, code: row.code, name: row.name })
}
function editRegion(row: TerritoryRegion) {
  regionTab.value = 'region'
  Object.assign(regionForm, { id: row.id, code: row.code, name: row.name, bigRegionId: row.bigRegionId })
}

async function saveBig() {
  if (!bigForm.code.trim() || !bigForm.name.trim()) {
    ElMessage.warning('请填写大区编码与名称')
    return
  }
  await territoryApi.saveBigRegion({ code: bigForm.code, name: bigForm.name }, bigForm.id || undefined)
  ElMessage.success('已保存')
  resetBigForm()
  await loadTerritory()
}

async function saveRegion() {
  if (!regionForm.code.trim() || !regionForm.name.trim() || !regionForm.bigRegionId) {
    ElMessage.warning('请填写区域编码、名称并选择大区')
    return
  }
  await territoryApi.saveRegion(
    { code: regionForm.code, name: regionForm.name, bigRegionId: regionForm.bigRegionId },
    regionForm.id || undefined,
  )
  ElMessage.success('已保存')
  resetRegionForm()
  await loadTerritory()
}

async function removeBig(row: BigRegion) {
  await ElMessageBox.confirm(`确定删除大区「${row.name}」吗？`, '删除确认', { type: 'warning' })
  await territoryApi.removeBigRegion(row.id)
  ElMessage.success('已删除')
  await loadTerritory()
}

async function removeRegion(row: TerritoryRegion) {
  await ElMessageBox.confirm(`确定删除区域「${row.name}」吗？`, '删除确认', { type: 'warning' })
  await territoryApi.removeRegion(row.id)
  ElMessage.success('已删除')
  await loadTerritory()
}

// ---------- Import / export ----------

const fileInput = ref<HTMLInputElement>()
const importDialog = ref(false)
const importResult = ref<ImportResult | null>(null)
let pendingMode: 'import' | 'report' = 'import'

function onBulk(cmd: string) {
  switch (cmd) {
    case 'template':
      territoryApi.storeTemplate()
      break
    case 'export':
      territoryApi.exportStores()
      break
    case 'regions':
      regionDialog.value = true
      loadTerritory()
      break
    case 'import':
    case 'report':
      pendingMode = cmd
      fileInput.value?.click()
      break
  }
}

async function onFilePicked(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (pendingMode === 'report') {
    await territoryApi.storeImportReport(file)
    return
  }
  const res = await territoryApi.importStores(file)
  importResult.value = res
  importDialog.value = true
  await load()
}

onMounted(async () => {
  await loadTerritory()
  await load()
})
</script>

<style scoped>
.filter-input {
  width: 210px;
}
.filter-select {
  width: 150px;
}
.full {
  width: 100%;
}
.code {
  margin-left: 6px;
}
.pager {
  margin-top: 12px;
}
@media (max-width: 640px) {
  .filter-input,
  .filter-select {
    width: 100%;
  }
}
</style>
