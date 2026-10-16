<template>
  <div>
    <el-alert title="网络策略可由多个用户组引用。用户名与来源 IP 同时满足才匹配；访问规则按顺序匹配，不得超出最终下发路由范围（用户策略优先于组路由）。修改后重连生效。" type="info" :closable="false"></el-alert>
    <el-button type="primary" @click="edit()" style="margin: 15px 0">新增网络策略</el-button>
    <el-table :data="tableData" border>
      <el-table-column prop="name" label="网络策略"></el-table-column>
      <el-table-column label="用户名"><template slot-scope="scope">{{ scope.row.usernames.join('、') || '不限制' }}</template></el-table-column>
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
    <el-dialog title="网络策略" :visible.sync="dialog" width="950px" :close-on-click-modal="false">
      <el-form label-width="110px" @submit.native.prevent>
        <el-form-item label="名称" required><el-input v-model="form.name" maxlength="60"></el-input></el-form-item>
        <el-form-item label="适用用户">
          <el-select v-model="selectedGroups" multiple filterable clearable placeholder="按用户组筛选，可多选" style="width: 100%">
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
        <el-form-item><span>用户名和来源 IP 至少填写一项。在用户组的权限控制中关联，多个网络策略按关联顺序执行。</span></el-form-item>
      </el-form>
      <span slot="footer"><el-button @click="dialog = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></span>
    </el-dialog>
  </div>
</template>

<script>
import axios from 'axios'

export default {
  name: 'NetworkPolicyList',
  data() {
    return {tableData: [], dialog: false, saving: false, form: {usernames: [], rules: []}, sourceIPs: '', userOptions: [], usersLoading: false, userSearchID: 0, userQuery: '', selectedGroups: []}
  },
  computed: {
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
    getData() {
      axios.get('/network_policy/list').then(resp => {
        if (resp.data.code === 0) this.tableData = resp.data.data.datas || []
        else this.$message.error(resp.data.msg)
      }).catch(() => this.$message.error('网络策略加载失败'))
    },
    edit(row) {
      this.form = row ? JSON.parse(JSON.stringify(row)) : {name: '', note: '', usernames: [], rules: []}
      this.$set(this.form, 'usernames', this.form.usernames || [])
      this.$set(this.form, 'rules', this.form.rules || [])
      this.userOptions = []
      this.userQuery = ''
      this.selectedGroups = []
      this.searchUsers()
      this.sourceIPs = (this.form.source_ips || []).join('\n')
      this.dialog = true
    },
    moveRule(index, offset) {
      const rule = this.form.rules.splice(index, 1)[0]
      this.form.rules.splice(index + offset, 0, rule)
    },
    selectFilteredUsers() {
      this.form.usernames = [...new Set(this.form.usernames.concat(this.filteredUsers.map(user => user.username)))]
    },
    searchUsers() {
      const id = ++this.userSearchID
      this.usersLoading = true
      axios.get('/network_policy/users').then(resp => {
        if (id !== this.userSearchID) return
        if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return }
        this.userOptions = resp.data.data || []
      }).catch(() => this.$message.error('用户加载失败')).finally(() => { if (id === this.userSearchID) this.usersLoading = false })
    },
    values(text) {
      return text.split(/[\n,，]+/).map(value => value.trim()).filter(Boolean)
    },
    save() {
      const data = Object.assign({}, this.form, {source_ips: this.values(this.sourceIPs)})
      if (!data.name.trim() || !data.rules.length || (!data.usernames.length && !data.source_ips.length)) {
        this.$message.error('请填写名称、访问规则，以及用户名或来源 IP')
        return
      }
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
