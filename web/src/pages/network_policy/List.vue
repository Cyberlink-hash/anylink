<template>
  <div>
    <el-alert title="网络策略可由多个用户组引用。用户名与来源 IP 同时满足才匹配；同一列表中任意一项匹配即可。修改后重连生效。" type="info" :closable="false"></el-alert>
    <el-button type="primary" @click="edit()" style="margin: 15px 0">新增网络策略</el-button>
    <el-table :data="tableData" border>
      <el-table-column prop="name" label="网络策略"></el-table-column>
      <el-table-column label="用户名"><template slot-scope="scope">{{ scope.row.usernames.join('、') || '不限制' }}</template></el-table-column>
      <el-table-column label="连接来源 IP"><template slot-scope="scope">{{ scope.row.source_ips.join('、') || '不限制' }}</template></el-table-column>
      <el-table-column prop="note" label="备注"></el-table-column>
      <el-table-column label="操作" width="160">
        <template slot-scope="scope">
          <el-button type="text" @click="edit(scope.row)">编辑</el-button>
          <el-button type="text" @click="remove(scope.row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-dialog title="网络策略" :visible.sync="dialog" width="620px" :close-on-click-modal="false">
      <el-form label-width="110px" @submit.native.prevent>
        <el-form-item label="名称" required><el-input v-model="form.name" maxlength="60"></el-input></el-form-item>
        <el-form-item label="用户名"><el-input type="textarea" :rows="4" v-model="usernames" placeholder="LDAP 登录用户名，每行一个或逗号分隔；留空不限制，按大小写精确匹配"></el-input></el-form-item>
        <el-form-item label="连接来源 IP"><el-input type="textarea" :rows="4" v-model="sourceIPs" placeholder="服务端看到的 VPN 连接来源 IP 或 CIDR 网段，每行一个或逗号分隔；留空不限制"></el-input></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.note" maxlength="255"></el-input></el-form-item>
        <el-form-item><span>用户名和来源 IP 至少填写一项。在用户组的权限控制规则中引用。</span></el-form-item>
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
    return {tableData: [], dialog: false, saving: false, form: {}, usernames: '', sourceIPs: ''}
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
      this.form = row ? JSON.parse(JSON.stringify(row)) : {name: '', note: ''}
      this.usernames = (this.form.usernames || []).join('\n')
      this.sourceIPs = (this.form.source_ips || []).join('\n')
      this.dialog = true
    },
    values(text) {
      return text.split(/[\n,，]+/).map(value => value.trim()).filter(Boolean)
    },
    save() {
      const data = Object.assign({}, this.form, {usernames: this.values(this.usernames), source_ips: this.values(this.sourceIPs)})
      if (!data.name.trim() || (!data.usernames.length && !data.source_ips.length)) {
        this.$message.error('请填写名称，以及用户名或来源 IP')
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
