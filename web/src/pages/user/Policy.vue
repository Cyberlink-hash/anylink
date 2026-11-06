<template>
  <div>
    <el-card>
      <el-alert type="info" :closable="false" title="指定组策略优先于所有组策略；继承项直接使用登录组配置。停用指定组策略会回退到所有组策略。修改在重新建立隧道后生效，不主动断开在线用户。"></el-alert>
      <el-form :inline="true" style="margin-top:16px" @submit.native.prevent="getData(1)">
        <el-form-item><el-button size="small" type="primary" @click="edit(null)">添加连接策略</el-button><el-button size="small" @click="openPreview()">预览最终配置</el-button></el-form-item>
        <el-form-item label="用户名"><el-input v-model="search" clearable size="small"></el-input></el-form-item>
        <el-form-item label="生效范围"><el-select v-model="scopeFilter" clearable size="small" placeholder="全部范围" @change="getData(1)"><el-option label="所有用户组" :value="0"></el-option><el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id"></el-option></el-select></el-form-item>
        <el-form-item><el-button size="small" @click="getData(1)">搜索</el-button></el-form-item>
      </el-form>
      <el-table :data="rows" border v-loading="loading">
        <el-table-column prop="id" label="ID" width="65"></el-table-column>
        <el-table-column prop="username" label="用户名" min-width="130"></el-table-column>
        <el-table-column label="生效范围" min-width="140"><template slot-scope="{ row }">{{ scopeName(row.group_id) }}<div v-if="!row.config_version"><el-tag size="mini" type="warning">旧版兼容模式</el-tag></div></template></el-table-column>
        <el-table-column v-for="module in modules" :key="module.key" :label="module.label" min-width="110"><template slot-scope="{ row }">{{ !row.config_version ? '旧版覆盖规则' : row[module.key + '_mode'] === 'custom' ? '自定义' : '继承登录组' }}</template></el-table-column>
        <el-table-column label="状态" width="80"><template slot-scope="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
        <el-table-column prop="updated_at" label="更新时间" :formatter="tableDateFormat" width="160"></el-table-column>
        <el-table-column label="操作" width="190"><template slot-scope="{ row }"><el-button type="text" @click="edit(row)">编辑</el-button><el-button type="text" @click="openPreview(row)">预览</el-button><el-button type="text" @click="remove(row)">删除</el-button></template></el-table-column>
      </el-table>
      <el-pagination :current-page="page" :page-size="pageSize" :total="count" layout="total, prev, pager, next" @current-change="getData" style="margin-top:16px"></el-pagination>
    </el-card>

    <el-dialog title="用户连接策略" :visible.sync="editing" width="850px" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
      <el-alert v-if="!form.config_version" type="warning" :closable="false" title="旧版兼容模式保留原有所有组覆盖行为，包括 DNS 的特殊继承规则。转换后请预览并核对 DNS 和路由。"><el-button type="text" @click="convertLegacy">转换为可继承配置</el-button></el-alert>
      <el-form label-width="125px" style="margin-top:16px">
        <el-form-item label="用户"><el-select v-model="form.username" filterable :disabled="!!form.id" placeholder="请选择用户" @change="form.group_id = null"><el-option v-for="u in users" :key="u.username" :value="u.username" :label="userLabel(u)"></el-option></el-select></el-form-item>
        <el-form-item label="生效范围"><el-select v-model="form.group_id" :disabled="!form.config_version" placeholder="请选择登录用户组"><el-option label="所有用户组（明确选择）" :value="0"></el-option><el-option v-for="g in userGroups(form.username)" :key="g.id" :value="g.id" :label="g.name"></el-option></el-select><div class="help">同一用户每个范围仅一份策略。指定组策略中的继承项不会叠加所有组策略。</div></el-form-item>
        <el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio :label="1">启用</el-radio><el-radio :label="0">停用</el-radio></el-radio-group></el-form-item>
        <el-tabs v-model="activeTab">
          <el-tab-pane v-for="module in modules" :key="module.key" :label="module.label" :name="module.key">
            <el-form-item v-if="form.config_version" label="配置方式"><el-radio-group v-model="form[module.key + '_mode']"><el-radio label="inherit">继承登录用户组</el-radio><el-radio label="custom">自定义</el-radio></el-radio-group></el-form-item>
            <p v-if="form.config_version && form[module.key + '_mode'] === 'inherit'" class="help">此模块直接跟随登录用户组；组配置修改后，下次建立隧道会使用新配置。</p>
            <template v-else>
              <el-form-item v-if="module.key === 'lan'" label="允许本地网络"><el-switch v-model="form.allow_lan"></el-switch></el-form-item>
              <template v-if="module.key === 'dns'"><el-form-item label="DNS 模式"><el-radio-group v-model="form.no_global_dns"><el-radio :label="false">全局 DNS</el-radio><el-radio :label="true">仅内网域名</el-radio></el-radio-group><div class="help">自定义 DNS 会替换该模块全部配置；全局 DNS 与所有流量走 VPN 是不同设置。</div></el-form-item></template>
              <template v-if="module.key === 'route'"><el-alert type="info" :closable="false" title="包含路由填写 all 表示所有流量走 VPN；填写网段表示分流。必须明确填写，不能用空白表示继承或全隧道。"></el-alert></template>
              <el-form-item v-for="field in module.fields.filter(f => f.key !== 'split_dns' || form.no_global_dns)" :key="field.key" :label="field.label">
                <div v-for="(item, index) in form[field.key]" :key="index" class="value-row"><el-input v-model="item.val" :placeholder="field.placeholder" size="small"></el-input><el-input v-model="item.note" placeholder="备注" size="small"></el-input><el-button size="mini" type="text" @click="form[field.key].splice(index, 1)">移除</el-button></div>
                <el-button size="small" @click="form[field.key].push({ val: '', note: '' })">添加</el-button>
              </el-form-item>
              <template v-if="module.key === 'domain'"><el-form-item label="包含域名"><el-input type="textarea" :rows="4" v-model="form.ds_include_domains"></el-input></el-form-item><el-form-item label="排除域名"><el-input type="textarea" :rows="4" v-model="form.ds_exclude_domains"></el-input></el-form-item><p class="help">包含和排除域名不能同时填写；自定义且两项为空表示清除组内的动态拆分设置。</p></template>
            </template>
          </el-tab-pane>
        </el-tabs>
      </el-form>
      <span slot="footer"><el-button @click="editing = false" :disabled="saving">取消</el-button><el-button @click="openPreview(form, true)" :disabled="saving">预览未保存配置</el-button><el-button type="primary" @click="save" :loading="saving">保存</el-button></span>
    </el-dialog>

    <el-dialog title="最终连接配置预览" :visible.sync="previewVisible" width="900px" append-to-body :close-on-click-modal="false">
      <el-alert type="info" :closable="false" :title="previewDraft ? '正在预览未保存配置，不会修改配置或影响连接。' : '正在预览已保存配置，不会修改配置或影响连接。'"></el-alert>
      <el-form :inline="true" style="margin-top:16px">
        <el-form-item label="用户"><el-select v-model="previewUser" filterable :disabled="!!previewDraft" @change="previewGroup = null; preview = null"><el-option v-for="u in users" :key="u.username" :value="u.username" :label="userLabel(u)"></el-option></el-select></el-form-item>
        <el-form-item label="登录用户组"><el-select v-model="previewGroup" @change="preview = null"><el-option v-for="g in userGroups(previewUser)" :key="g.id" :label="g.name" :value="g.id"></el-option></el-select></el-form-item>
        <el-form-item><el-button type="primary" @click="loadPreview" :loading="previewLoading" :disabled="!previewUser || !previewGroup">查看最终配置</el-button></el-form-item>
      </el-form>
      <div v-if="preview">
        <el-alert v-for="warning in preview.warnings" :key="warning" :title="warning" type="warning" :closable="false"></el-alert>
        <p>匹配策略：{{ preview.policy ? '#' + preview.policy.id + ' · ' + scopeName(preview.policy.group_id) : '无，使用用户组默认配置' }}<span v-if="preview.policy && !preview.policy.id">（未保存）</span></p>
        <el-table :data="previewRows" border><el-table-column prop="label" label="配置" width="160"></el-table-column><el-table-column prop="value" label="最终值" min-width="350"><template slot-scope="{ row }"><span style="white-space:pre-line">{{ row.value }}</span></template></el-table-column><el-table-column prop="source" label="来源" min-width="170"></el-table-column></el-table>
        <p>组关联网络访问策略：{{ preview.network_policies.map(p => p.name).join('、') || '无' }}；组内访问规则 {{ preview.link_acl ? preview.link_acl.length : 0 }} 条。</p>
        <p class="help">预览为策略合并结果，客户端出口排除路由等会话参数在连接时生成。这里只展示组关联关系，不判断某个来源 IP 是否命中，也不代表目标地址和端口一定可访问。路由和 DNS 配置不会覆盖服务端访问规则。在线连接可能仍在使用旧配置，以重新建立隧道后的配置为准。</p>
      </div>
    </el-dialog>
  </div>
</template>

<script>
import axios from 'axios';

const newForm = () => ({ id: 0, username: '', group_id: null, config_version: 1, status: 1, lan_mode: 'inherit', dns_mode: 'inherit', route_mode: 'inherit', domain_mode: 'inherit', allow_lan: false, no_global_dns: false, client_dns: [], split_dns: [], route_include: [], route_exclude: [], ds_include_domains: '', ds_exclude_domains: '' });
export default {
  name: 'Policy',
  data() {
    return { rows: [], count: 0, page: 1, pageSize: 10, search: '', scopeFilter: '', groups: [], users: [], loading: false, requestID: 0, editing: false, saving: false, form: newForm(), activeTab: 'route', previewVisible: false, previewLoading: false, previewRequestID: 0, preview: null, previewUser: '', previewGroup: null, previewDraft: null,
      modules: [
        { key: 'route', label: '路由', fields: [{ key: 'route_include', label: '包含路由', placeholder: '10.0.0.0/24 或 all' }, { key: 'route_exclude', label: '排除路由', placeholder: '192.168.0.0/24' }] },
        { key: 'dns', label: 'DNS', fields: [{ key: 'client_dns', label: '客户端 DNS', placeholder: '10.0.0.53' }, { key: 'split_dns', label: '内网域名', placeholder: 'example.com' }] },
        { key: 'lan', label: '本地网络', fields: [] }, { key: 'domain', label: '动态拆分域名', fields: [] },
      ] };
  },
  computed: {
    previewRows() {
      const p = this.preview; if (!p) { return []; }
      const vals = list => (list || []).map(v => v.val).join('\n') || '无';
      const full = !p.route_include || !p.route_include.length || p.route_include.some(v => v.val === 'all');
      return [
        { label: '连接方式', value: full ? '全隧道：所有流量走 VPN（排除路由除外）' : '分流：仅包含路由走 VPN', source: p.sources.route },
        { label: '包含路由', value: full ? 'all' : vals(p.route_include), source: p.sources.route }, { label: '排除路由', value: vals(p.route_exclude), source: p.sources.route },
        { label: '客户端 DNS', value: vals(p.client_dns), source: p.sources.dns }, { label: 'DNS 模式 / 内网域名', value: p.no_global_dns ? '仅内网域名\n' + vals(p.split_dns) : '全局 DNS', source: p.sources.split_dns },
        { label: '允许本地网络', value: p.allow_lan ? '是' : '否', source: p.sources.lan }, { label: '动态包含域名', value: p.ds_include_domains || '无', source: p.sources.domain }, { label: '动态排除域名', value: p.ds_exclude_domains || '无', source: p.sources.domain },
      ];
    },
  },
  created() { this.$emit('update:route_path', this.$route.path); this.$emit('update:route_name', ['用户信息', '用户连接策略']); this.loadOptions(); this.getData(1); },
  beforeDestroy() { this.requestID++; this.previewRequestID++; },
  methods: {
    userLabel(u) { return u.username + (u.nickname ? ' · ' + u.nickname : ''); },
    userGroups(username) { const u = this.users.find(u => u.username === username); return u ? this.groups.filter(g => (u.groups || []).includes(g.name)) : []; },
    scopeName(id) { const g = this.groups.find(g => g.id === id); return id === 0 ? '所有用户组' : g ? g.name : '用户组已不存在 #' + id; },
    async loadOptions() {
      try {
        const results = await Promise.all([axios.get('/group/names_ids'), axios.get('/network_policy/users')]);
        for (const result of results) { if (result.data.code !== 0) { throw new Error(result.data.msg); } }
        this.groups = results[0].data.data.datas || []; this.users = results[1].data.data || [];
      } catch (e) { this.$message.error(e.message || '用户和用户组加载失败'); }
    },
    async getData(page = 1) {
      const id = ++this.requestID; this.loading = true;
      try {
        const { data } = await axios.get('/user/policy/list', { params: { page, search: this.search, group_id: this.scopeFilter } });
        if (id !== this.requestID) { return; } if (data.code !== 0) { throw new Error(data.msg); }
        this.rows = data.data.datas || []; this.count = data.data.count; this.pageSize = data.data.page_size; this.page = page;
        if (!this.rows.length && page > 1) { await this.getData(Math.max(1, Math.ceil(this.count / this.pageSize))); }
      } catch (e) { if (id === this.requestID) { this.$message.error(e.message || '策略列表加载失败'); } }
      finally { if (id === this.requestID) { this.loading = false; } }
    },
    async edit(row) {
      try {
        let form = newForm();
        if (row) { const { data } = await axios.get('/user/policy/detail', { params: { id: row.id } }); if (data.code !== 0) { throw new Error(data.msg); } form = data.data; }
        for (const key of ['client_dns', 'split_dns', 'route_include', 'route_exclude']) { form[key] = form[key] || []; }
        this.form = form; this.activeTab = 'route'; this.editing = true;
      } catch (e) { this.$message.error(e.message || '策略加载失败'); }
    },
    convertLegacy() {
      this.$confirm('转换后四个模块暂设为自定义。DNS 将完整使用本策略，不再保留旧版的部分继承行为；请逐项选择继承或自定义并预览后保存。', '转换旧策略').then(() => {
        this.form.config_version = 1;
        for (const module of this.modules) { this.$set(this.form, module.key + '_mode', 'custom'); }
        if (!this.form.route_include.length) { this.form.route_include = [{ val: 'all', note: '' }]; }
      }).catch(() => {});
    },
    draft() {
      if (!this.form.username || this.form.group_id === null || this.form.group_id === '') { throw new Error('请选择用户和生效范围'); }
      const value = JSON.parse(JSON.stringify(this.form)); if (!value.no_global_dns) { value.split_dns = []; } return value;
    },
    async save() {
      if (this.saving) { return; }
      try {
        const value = this.draft();
        const summary = this.modules.map(m => m.label + '：' + (!value.config_version ? '旧版覆盖' : value[m.key + '_mode'] === 'inherit' ? '继承登录组' : '自定义')).join('；');
        await this.$confirm('用户 ' + value.username + '，范围：' + this.scopeName(value.group_id) + '。' + summary + '。保存后重新建立隧道生效，不会主动断开在线连接。', '确认策略变更');
        this.saving = true;
        const { data } = await axios.post('/user/policy/set', value); if (data.code !== 0) { throw new Error(data.msg); }
        this.$message.success('策略已保存，重新建立隧道后生效'); this.editing = false; await this.getData(this.page);
      } catch (e) { if (e !== 'cancel' && e !== 'close') { this.$message.error(e.message || '保存失败'); } }
      finally { this.saving = false; }
    },
    async remove(row) {
      try { await this.$confirm('删除后将回退到其他匹配策略或用户组默认配置，重新建立隧道后生效。确认删除？', '删除策略'); const { data } = await axios.post('/user/policy/del', null, { params: { id: row.id } }); if (data.code !== 0) { throw new Error(data.msg); } await this.getData(this.page); }
      catch (e) { if (e !== 'cancel' && e !== 'close') { this.$message.error(e.message || '删除失败'); } }
    },
    openPreview(row, unsaved = false) {
      try { this.previewRequestID++; this.previewLoading = false; this.previewDraft = unsaved ? this.draft() : null; this.previewUser = row ? row.username : ''; this.previewGroup = row && row.group_id ? row.group_id : null; this.preview = null; this.previewVisible = true; }
      catch (e) { this.$message.error(e.message); }
    },
    async loadPreview() {
      const id = ++this.previewRequestID; this.previewLoading = true; this.preview = null;
      const username = this.previewUser; const group = this.previewGroup;
      try { const { data } = await axios.post('/user/policy/preview', { username, group_id: group, policy: this.previewDraft }); if (id !== this.previewRequestID || username !== this.previewUser || group !== this.previewGroup) { return; } if (data.code !== 0) { throw new Error(data.msg); } this.preview = data.data; }
      catch (e) { if (id === this.previewRequestID) { this.$message.error(e.message || '预览失败'); } }
      finally { if (id === this.previewRequestID) { this.previewLoading = false; } }
    },
  },
};
</script>

<style scoped>
.help { color: #909399; font-size: 12px; line-height: 1.7; }
.value-row { display: flex; gap: 8px; margin-bottom: 8px; }
.value-row .el-button { flex-shrink: 0; }
</style>
