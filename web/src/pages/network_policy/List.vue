<template>
  <div>
    <el-alert title="在本页面统一选择策略关联的登录用户组。用户名与来源 IP 同时满足才匹配；访问规则按顺序匹配，不得超出最终下发路由范围（按用户连接策略的覆盖或继承结果）。修改后重连生效。" type="info" :closable="false"></el-alert>
    <el-button type="primary" @click="edit()" style="margin: 15px 0">新增网络策略</el-button>
    <el-table :data="tableData" border>
      <el-table-column prop="name" label="网络策略"></el-table-column>
      <el-table-column label="关联用户组" min-width="210"><template slot-scope="{ row }">
        <div v-for="group in row.groups" :key="group.id">{{ group.name }}<span v-if="group.order">（顺序 {{ group.order }}）</span><el-tag v-if="group.legacy" size="mini" type="warning">旧版条件引用</el-tag></div>
        <span v-if="!row.groups || !row.groups.length">未关联</span>
      </template></el-table-column>
      <el-table-column label="适用用户"><template slot-scope="{ row }"><template v-if="row.user_mode === 'group_members'">关联组全部成员（动态）<div>排除：{{ (row.excluded_usernames || []).join('、') || '无' }}</div></template><template v-else>旧版：{{ (row.usernames || []).join('、') || '不限制用户名' }}</template></template></el-table-column>
      <el-table-column label="连接来源 IP"><template slot-scope="scope">{{ scope.row.source_ips.join('、') || '不限制' }}</template></el-table-column>
      <el-table-column label="访问规则"><template slot-scope="scope"><div v-for="(rule,index) in scope.row.rules" :key="index">{{ rule.action === 'allow' ? '允许' : '拒绝' }} {{ rule.val }} {{ rule.protocol }} / {{ rule.port }}</div><span v-if="!scope.row.rules || !scope.row.rules.length">旧版条件策略，请补充访问规则</span></template></el-table-column>
      <el-table-column prop="note" label="备注"></el-table-column>
      <el-table-column label="操作" width="160">
        <template slot-scope="scope">
          <el-button type="text" @click="edit(scope.row)">编辑</el-button>
          <el-button type="text" @click="remove(scope.row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-dialog title="网络策略" :visible.sync="dialog" width="950px" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving">
      <el-form label-width="110px" @submit.native.prevent>
        <el-form-item label="名称" required><el-input v-model="form.name" maxlength="60"></el-input></el-form-item>
        <el-form-item label="关联登录组">
          <el-input v-model="groupQuery" clearable placeholder="搜索要关联的用户组" size="small"></el-input>
          <div style="margin: 8px 0"><el-button size="small" :disabled="groupsLoading || !filteredGroups.length" @click="selectFilteredGroups">全选当前筛选结果（{{ filteredGroups.length }}）</el-button><el-button size="small" :disabled="!form.group_ids.length" @click="form.group_ids = []">清空直接关联</el-button><span> 已选 {{ form.group_ids.length }} 个组</span></div>
          <el-select v-model="form.group_ids" multiple filterable :loading="groupsLoading" :disabled="groupsLoading" placeholder="选择实际登录用户组；留空不直接关联任何组" style="width:100%">
            <el-option v-for="group in groupOptions" :key="group.id" :value="group.id" :label="group.name" v-show="!groupQuery || group.name.toLowerCase().includes(groupQuery.trim().toLowerCase()) || form.group_ids.includes(group.id)"></el-option>
          </el-select>
          <div>选择关联组后，下方自动显示成员并集，跨组重合用户只显示一次。新关联追加到组内网络策略末尾；已有顺序不变，顺序可在用户组权限控制中调整。以后新建的组不会自动关联。</div>
          <el-alert v-if="legacyGroups.length" type="warning" :closable="false" :title="'旧版条件引用：' + legacyGroups.map(g => g.name).join('、') + '。取消直接关联不会删除这些组内的权限规则；需要解除时请在对应组的权限控制中处理。'"></el-alert>
        </el-form-item>
        <el-form-item v-if="form.user_mode === 'group_members'" label="适用用户">
          <div>已纳入 {{ selectedMemberCount }} / {{ memberUsers.length }} 人（按组成员并集去重）。新增或同步进组的用户自动纳入；取消勾选后加入排除名单。</div>
          <el-input v-model="userQuery" clearable placeholder="搜索用户名或姓名，不改变勾选状态" style="margin:8px 0"></el-input>
          <el-button size="small" :disabled="usersLoading || !filteredMembers.length" @click="setFilteredMembers(true)">勾选当前筛选结果</el-button>
          <el-button size="small" :disabled="usersLoading || !filteredMembers.length" @click="setFilteredMembers(false)">取消当前筛选结果</el-button>
          <el-table :data="filteredMembers" v-loading="usersLoading" max-height="330" border style="margin-top:8px" empty-text="所选组暂无匹配成员">
            <el-table-column label="适用" width="65"><template slot-scope="{ row }"><el-checkbox :value="!form.excluded_usernames.includes(row.username)" @change="toggleMember(row.username, $event)"></el-checkbox></template></el-table-column>
            <el-table-column prop="username" label="用户名"></el-table-column><el-table-column prop="nickname" label="姓名"></el-table-column>
            <el-table-column label="所属关联组"><template slot-scope="{ row }">{{ (row.groups || []).filter(name => effectiveGroupNames.includes(name)).join('、') }}</template></el-table-column>
          </el-table>
          <div>排除名单（{{ form.excluded_usernames.length }} 人）：<span v-if="!form.excluded_usernames.length">无</span><el-tag v-for="name in form.excluded_usernames" :key="name" closable @close="toggleMember(name, true)" style="margin:4px">{{ name }}{{ memberUsers.some(u => u.username === name) ? '' : '（当前不在关联组）' }}</el-tag></div>
          <div>取消勾选只让该用户跳过本策略，后续策略仍会匹配。排除名单不会因移组或重复选组被清空；重新勾选或移除排除标签可恢复。若存在旧版条件引用，对应组成员也计入并集。变更在重连后生效。</div>
        </el-form-item>
        <el-form-item v-else label="旧版适用用户">
          <el-alert title="旧策略保留原有固定用户名限制。转换为组成员模式后，后续新增成员会自动生效。" type="warning" :closable="false"></el-alert>
          <el-button type="primary" size="small" @click="convertMembers" :disabled="usersLoading || !usersReady || !groupsReady">使用关联组成员</el-button>
          <el-select v-model="selectedGroups" multiple filterable clearable placeholder="按成员所属组筛选用户（不代表关联策略）" style="width: 100%">
            <el-option v-for="group in userGroups" :key="group" :value="group" :label="group"></el-option>
          </el-select>
          <el-input v-model="userQuery" clearable placeholder="搜索用户名或姓名" style="margin: 8px 0"></el-input>
          <div>
            <el-button size="small" :disabled="usersLoading || !filteredUsers.length" @click="selectFilteredUsers">全选当前筛选结果（{{ filteredUsers.length }}）</el-button>
            <el-button size="small" :disabled="!form.usernames.length" @click="form.usernames = []">清空已选</el-button>
            <span>已选 {{ form.usernames.length }} 人</span>
          </div>
          <el-select v-model="form.usernames" multiple collapse-tags :loading="usersLoading" placeholder="选择用户；留空不限制" style="width: 100%; margin-top: 8px">
            <el-option v-for="user in selectableUsers" :key="user.username" :value="user.username" :label="user.nickname ? user.username + '（' + user.nickname + '）' : user.username" v-show="filteredUsers.some(item => item.username === user.username)"></el-option>
          </el-select>
          <div>按组筛选后可逐个选择成员，也可全选；多个组取并集，切换筛选保留已选用户。以后新增的用户需重新选择。清空名单表示不限制用户名。</div>
        </el-form-item>
        <el-form-item label="连接来源 IP"><el-input type="textarea" :rows="4" v-model="sourceIPs" placeholder="服务端看到的 VPN 连接来源 IP 或 CIDR 网段，每行一个或逗号分隔；留空不限制"></el-input></el-form-item>
        <el-form-item label="访问规则" required>
          <div>第一条目标匹配的规则生效。来源不匹配会跳过本策略；限制来源时，请再关联一条不限制来源的拒绝策略兜底。</div>
          <el-table :data="form.rules" border>
            <el-table-column label="动作" width="95"><template slot-scope="scope"><el-select v-model="scope.row.action"><el-option label="允许" value="allow"></el-option><el-option label="拒绝" value="deny"></el-option></el-select></template></el-table-column>
            <el-table-column label="目标 IP / 网段"><template slot-scope="scope"><el-input v-model="scope.row.val" placeholder="10.30.0.10 或 10.30.0.0/24"></el-input></template></el-table-column>
            <el-table-column label="协议" width="100"><template slot-scope="scope"><el-select v-model="scope.row.protocol"><el-option v-for="protocol in ['all','tcp','udp','icmp']" :key="protocol" :label="protocol" :value="protocol"></el-option></el-select></template></el-table-column>
            <el-table-column label="端口（0为全部）" width="150"><template slot-scope="scope"><el-input v-model="scope.row.port" :disabled="scope.row.protocol === 'icmp'" placeholder="443 或 80,443"></el-input></template></el-table-column>
            <el-table-column label="顺序 / 删除" width="125"><template slot-scope="scope"><el-button type="text" :disabled="scope.$index === 0" @click="moveRule(scope.$index,-1)">↑</el-button><el-button type="text" :disabled="scope.$index === form.rules.length-1" @click="moveRule(scope.$index,1)">↓</el-button><el-button type="text" @click="form.rules.splice(scope.$index,1)">删除</el-button></template></el-table-column>
          </el-table>
          <el-button size="small" @click="form.rules.push({action: 'allow', val: '', protocol: 'tcp', port: '443'})">添加规则</el-button>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="form.note" maxlength="255"></el-input></el-form-item>
        <el-form-item><span v-if="form.user_mode === 'group_members'">来源 IP 留空表示不限制来源。组内成员默认全选，仅保存取消勾选的排除名单；没有任何关联或引用时策略不生效。</span><span v-else>旧版模式仍要求用户名和来源 IP 至少填写一项。</span></el-form-item>
      </el-form>
      <span slot="footer"><el-button @click="dialog = false" :disabled="saving">取消</el-button><el-button type="primary" :loading="saving" :disabled="groupsLoading || !groupsReady || usersLoading || !usersReady" @click="save">保存</el-button></span>
    </el-dialog>
  </div>
</template>

<script>
import axios from 'axios'

export default {
  name: 'NetworkPolicyList',
  data() {
    return {tableData: [], dialog: false, saving: false, form: {usernames: [], rules: [], group_ids: [], user_mode: 'group_members', excluded_usernames: []}, sourceIPs: '', userOptions: [], usersLoading: false, usersReady: false, userSearchID: 0, userQuery: '', selectedGroups: [], groupOptions: [], groupQuery: '', groupsLoading: false, groupsReady: false}
  },
  computed: {
    effectiveGroupNames() {
      const ids = new Set((this.form.group_ids || []).concat(this.legacyGroups.map(g => g.id)))
      return this.groupOptions.filter(g => ids.has(g.id)).map(g => g.name)
    },
    memberUsers() {
      const seen = new Set()
      return this.userOptions.filter(user => {
        if (seen.has(user.username) || !(user.groups || []).some(g => this.effectiveGroupNames.includes(g))) return false
        seen.add(user.username); return true
      })
    },
    selectedMemberCount() { return this.memberUsers.filter(u => !this.form.excluded_usernames.includes(u.username)).length },
    filteredMembers() {
      const query = this.userQuery.trim().toLowerCase()
      return this.memberUsers.filter(u => !query || u.username.toLowerCase().includes(query) || (u.nickname || '').toLowerCase().includes(query))
    },
    filteredGroups() {
      const query = this.groupQuery.trim().toLowerCase()
      return this.groupOptions.filter(group => !query || group.name.toLowerCase().includes(query))
    },
    legacyGroups() { return (this.form.groups || []).filter(group => group.legacy) },
    userGroups() {
      return [...new Set(this.userOptions.flatMap(user => user.groups || []))].sort()
    },
    filteredUsers() {
      const query = this.userQuery.trim().toLowerCase()
      return this.userOptions.filter(user =>
        (!this.selectedGroups.length || (user.groups || []).some(group => this.selectedGroups.includes(group))) &&
        (!query || user.username.toLowerCase().includes(query) || (user.nickname || '').toLowerCase().includes(query)))
    },
    selectableUsers() {
      const missing = this.form.usernames.filter(username => !this.userOptions.some(user => user.username === username))
      return this.userOptions.concat(missing.map(username => ({username})))
    },
  },
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['网络策略组'])
    this.getData()
  },
  methods: {
    toggleMember(username, checked) {
      if (checked) this.form.excluded_usernames = this.form.excluded_usernames.filter(name => name !== username)
      else this.form.excluded_usernames = [...new Set(this.form.excluded_usernames.concat(username))]
    },
    setFilteredMembers(checked) {
      const names = new Set(this.filteredMembers.map(u => u.username))
      this.form.excluded_usernames = checked ? this.form.excluded_usernames.filter(name => !names.has(name)) : [...new Set(this.form.excluded_usernames.concat([...names]))]
    },
    convertMembers() {
      this.$confirm('转换后，当前组成员中未在原用户名名单内的用户会先列入排除名单；以后新增或同步进组的成员默认纳入。原来源 IP 条件保持，保存前请核对勾选。', '转换用户适用方式').then(() => {
        const old = this.form.usernames || []
        this.form.excluded_usernames = old.length ? this.memberUsers.filter(u => !old.includes(u.username)).map(u => u.username) : []
        this.form.usernames = []
        this.$set(this.form, 'user_mode', 'group_members')
      }).catch(() => {})
    },
    getData() {
      axios.get('/network_policy/list').then(resp => {
        if (resp.data.code === 0) this.tableData = resp.data.data.datas || []
        else this.$message.error(resp.data.msg)
      }).catch(() => this.$message.error('网络策略加载失败'))
    },
    async edit(row) {
      try {
        let data = {name: '', note: '', usernames: [], rules: [], group_ids: [], groups: [], user_mode: 'group_members', excluded_usernames: []}
        if (row) {
          const response = await axios.get('/network_policy/detail', {params: {id: row.id}})
          if (response.data.code !== 0) throw new Error(response.data.msg)
          data = response.data.data
        }
        this.form = data
        this.$set(this.form, 'usernames', this.form.usernames || [])
        this.$set(this.form, 'rules', this.form.rules || [])
        this.$set(this.form, 'excluded_usernames', this.form.excluded_usernames || [])
        this.$set(this.form, 'group_ids', this.form.group_ids || [])
        this.$set(this.form, 'original_group_ids', this.form.group_ids.slice())
        this.userOptions = []; this.userQuery = ''; this.selectedGroups = []; this.groupQuery = ''
        this.searchUsers(); this.loadGroups()
        this.sourceIPs = (this.form.source_ips || []).join('\n')
        this.dialog = true
      } catch (error) { this.$message.error(error.message || '网络策略加载失败') }
    },
    async loadGroups() {
      this.groupsLoading = true; this.groupsReady = false
      try {
        const {data} = await axios.get('/group/names_ids')
        if (data.code !== 0) throw new Error(data.msg)
        this.groupOptions = data.data.datas || []; this.groupsReady = true
      } catch (error) { this.$message.error(error.message || '用户组加载失败，请重新打开编辑') }
      finally { this.groupsLoading = false }
    },
    selectFilteredGroups() {
      this.form.group_ids = [...new Set(this.form.group_ids.concat(this.filteredGroups.map(group => group.id)))]
    },
    groupNames(ids) { return ids.map(id => { const group = this.groupOptions.find(g => g.id === id); return group ? group.name : '#' + id }).join('、') || '无' },
    moveRule(index, offset) {
      const rule = this.form.rules.splice(index, 1)[0]
      this.form.rules.splice(index + offset, 0, rule)
    },
    selectFilteredUsers() {
      this.form.usernames = [...new Set(this.form.usernames.concat(this.filteredUsers.map(user => user.username)))]
    },
    searchUsers() {
      const id = ++this.userSearchID
      this.usersLoading = true; this.usersReady = false
      axios.get('/network_policy/users').then(resp => {
        if (id !== this.userSearchID) return
        if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return }
        this.userOptions = resp.data.data || []; this.usersReady = true
      }).catch(() => this.$message.error('用户加载失败')).finally(() => { if (id === this.userSearchID) this.usersLoading = false })
    },
    values(text) {
      return text.split(/[\n,，]+/).map(value => value.trim()).filter(Boolean)
    },
    async save() {
      if (this.saving || this.groupsLoading || !this.groupsReady || this.usersLoading || !this.usersReady) return
      const data = Object.assign({}, this.form, {source_ips: this.values(this.sourceIPs)})
      if (!data.name.trim() || !data.rules.length || (data.user_mode !== 'group_members' && !data.usernames.length && !data.source_ips.length)) {
        this.$message.error('请填写名称、访问规则；旧版模式还需填写用户名或来源 IP')
        return
      }
      const added = data.group_ids.filter(id => !data.original_group_ids.includes(id))
      const removed = data.original_group_ids.filter(id => !data.group_ids.includes(id))
      try {
        if (data.user_mode === 'group_members' && this.memberUsers.length && !this.selectedMemberCount) await this.$confirm('当前组内所有成员都被排除，会跳过本策略；以后新增成员仍会自动纳入。确认保存？', '当前适用人数为零')
        if (added.length || removed.length) await this.$confirm('新增关联：' + this.groupNames(added) + '；解除直接关联：' + this.groupNames(removed) + '。已有顺序保持，新关联追加到末尾，重连后生效。', '确认关联变更')
      } catch (_) { return }
      this.saving = true
      axios.post('/network_policy/set', data).then(resp => {
        if (resp.data.code === 0) {
          this.dialog = false
          this.$message.success('已保存，重连后生效')
          this.getData()
        } else this.$message.error(resp.data.msg)
      }).catch(() => this.$message.error('保存失败')).finally(() => { this.saving = false })
    },
    remove(row) {
      this.$confirm('确定删除网络策略“' + row.name + '”？被用户组引用的策略不可删除。', '提示').then(() => {
        axios.post('/network_policy/del?id=' + row.id).then(resp => {
          if (resp.data.code === 0) this.getData()
          else this.$message.error(resp.data.msg)
        }).catch(() => this.$message.error('删除失败'))
      }).catch(() => {})
    },
  },
}
</script>
