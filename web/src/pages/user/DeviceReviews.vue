<template>
  <el-card>
    <el-alert type="info" :closable="false" title="请先通过可信方式与用户核实设备。批准设备不会自动开启该用户的设备绑定；开启或关闭绑定请进入用户设备管理。"></el-alert>
    <el-form :inline="true" @submit.native.prevent="load(1)" style="margin-top: 16px">
      <el-form-item label="用户名"><el-input v-model="search" size="small" clearable placeholder="搜索用户名"></el-input></el-form-item>
      <el-form-item label="设备状态"><el-select v-model="state" size="small" @change="load(1)"><el-option label="全部" value=""></el-option><el-option v-for="item in states" :key="item.value" :label="item.label" :value="item.value"></el-option></el-select></el-form-item>
      <el-form-item><el-button size="small" type="primary" @click="load(1)" :disabled="busy">搜索</el-button><el-button size="small" @click="load(page)" :disabled="busy">刷新</el-button></el-form-item>
    </el-form>
    <el-table :data="rows" border v-loading="loading" empty-text="暂无符合条件的设备记录">
      <el-table-column prop="id" label="申请号" width="85"></el-table-column>
      <el-table-column prop="username" label="用户名" min-width="120"></el-table-column>
      <el-table-column label="设备绑定" width="105"><template slot-scope="{ row }"><el-tag size="small" :type="row.binding_enabled ? 'success' : 'info'">{{ row.binding_enabled ? '已开启' : '未开启' }}</el-tag></template></el-table-column>
      <el-table-column label="设备 / 系统" min-width="170"><template slot-scope="{ row }"><div>{{ row.label || '未备注' }}</div><div>{{ row.device_type || '未知设备' }} {{ row.platform_version }}</div><small>{{ row.user_agent }}</small></template></el-table-column>
      <el-table-column label="标识摘要" min-width="170"><template slot-scope="{ row }"><div>{{ row.key_kind }}</div><el-tooltip :content="row.key_hash" placement="top"><span>{{ row.key_hash.slice(0, 16) }}…</span></el-tooltip></template></el-table-column>
      <el-table-column prop="remote_addr" label="最近来源 IP" min-width="130"></el-table-column>
      <el-table-column label="申请时间" width="160"><template slot-scope="{ row }">{{ row.requested_at ? tableDateFormat(row, { property: 'requested_at' }) : '未记录' }}</template></el-table-column>
      <el-table-column prop="last_seen" label="最近认证 / 连接" :formatter="tableDateFormat" width="160"></el-table-column>
      <el-table-column label="状态" width="95"><template slot-scope="{ row }">{{ stateLabel(row.state) }}</template></el-table-column>
      <el-table-column label="操作" min-width="225"><template slot-scope="{ row }">
        <template v-if="row.state === 'pending'">
          <el-button type="text" :disabled="busy" @click="confirmReview('approve', row)">批准新增</el-button>
          <el-button type="text" :disabled="busy" @click="openReplace(row)">替换旧设备</el-button>
          <el-button type="text" :disabled="busy" @click="confirmReview('reject', row)">拒绝</el-button>
        </template>
        <el-button type="text" :disabled="busy" @click="deviceUser = { id: row.user_id, username: row.username }">设备管理</el-button>
      </template></el-table-column>
    </el-table>
    <el-pagination style="margin-top: 16px" layout="total, prev, pager, next" :total="total" :current-page="page" :page-size="pageSize" @current-change="load"></el-pagination>
    <user-devices v-if="deviceUser" :user="deviceUser" :visible="true" @close="deviceUser = null" @changed="load(page)"></user-devices>
    <el-dialog title="批准新设备并替换旧设备" :visible.sync="replaceVisible" width="480px" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy">
      <p v-if="replacement">用户 {{ replacement.username }}，新设备 #{{ replacement.id }}。确认后所选旧设备将被撤销并断开连接。</p>
      <el-select v-model="replaceId" placeholder="选择已批准的旧设备" style="width:100%" :disabled="busy"><el-option v-for="device in approved" :key="device.id" :value="device.id" :label="'#' + device.id + ' ' + (device.label || device.device_type || '未备注设备')"></el-option></el-select>
      <span slot="footer"><el-button :disabled="busy" @click="replaceVisible = false">取消</el-button><el-button type="primary" :disabled="!replaceId || busy" @click="review('replace', replacement, replaceId)">确认替换</el-button></span>
    </el-dialog>
  </el-card>
</template>

<script>
import axios from 'axios';
import UserDevices from './Devices.vue';

export default {
  name: 'DeviceReviews',
  components: { UserDevices },
  data() {
    return { search: '', state: 'pending', rows: [], page: 1, pageSize: 10, total: 0, loading: false, busy: false, requestID: 0, deviceUser: null, replaceVisible: false, replacement: null, replaceId: null, approved: [],
      states: [{ value: 'pending', label: '待审批' }, { value: 'approved', label: '已批准' }, { value: 'rejected', label: '已拒绝' }, { value: 'revoked', label: '已撤销' }, { value: 'observed', label: '候选' }] };
  },
  created() {
    this.$emit('update:route_path', this.$route.path);
    this.$emit('update:route_name', ['用户信息', '设备审批']);
    this.load(1);
  },
  beforeDestroy() { this.requestID++; },
  methods: {
    stateLabel(value) { const state = this.states.find(item => item.value === value); return state ? state.label : value; },
    async load(page = 1) {
      const id = ++this.requestID;
      this.loading = true;
      try {
        const { data } = await axios.get('/user/device-reviews', { params: { page, search: this.search.trim(), state: this.state } });
        if (id !== this.requestID) { return; }
        if (data.code !== 0) { throw new Error(data.msg); }
        this.rows = data.data.datas || []; this.total = data.data.count; this.pageSize = data.data.page_size; this.page = page;
        if (!this.rows.length && page > 1) { await this.load(Math.max(1, Math.ceil(this.total / this.pageSize))); }
      } catch (error) { if (id === this.requestID) { this.$message.error(error.message || '审批列表加载失败'); } }
      finally { if (id === this.requestID) { this.loading = false; } }
    },
    confirmReview(action, row) {
      const message = action === 'approve' ? '确认已核实用户 ' + row.username + ' 的设备 #' + row.id + '，并批准新增？' : '拒绝用户 ' + row.username + ' 的设备申请 #' + row.id + '？';
      this.$confirm(message, '设备审批').then(() => this.review(action, row)).catch(() => {});
    },
    async openReplace(row) {
      if (this.busy) { return; }
      this.busy = true;
      try {
        const { data } = await axios.get('/user/devices', { params: { user_id: row.user_id } });
        if (data.code !== 0) { throw new Error(data.msg); }
        if (data.data.policy.revision !== row.revision) { await this.load(this.page); throw new Error('设备设置已变更，列表已刷新，请重新选择'); }
        this.approved = data.data.devices.filter(device => device.state === 'approved');
        if (!this.approved.length) { throw new Error('该用户没有已批准的旧设备，请使用“批准新增”'); }
        this.replacement = row; this.replaceId = null; this.replaceVisible = true;
      } catch (error) { this.$message.error(error.message || '读取旧设备失败'); }
      finally { this.busy = false; }
    },
    async review(action, row, replaceId = 0) {
      if (this.busy) { return; }
      this.busy = true;
      try {
        const { data } = await axios.post('/user/devices/change', { user_id: row.user_id, revision: row.revision, device_id: row.id, action, replace_id: replaceId });
        if (data.code !== 0) { throw new Error(data.msg); }
        this.$message.success('操作成功，已断开 ' + data.data.closed_sessions + ' 条连接');
        this.replaceVisible = false;
        window.dispatchEvent(new Event('user-devices-changed'));
      } catch (error) { this.$message.error(error.message || '审批失败，请刷新确认'); }
      finally { await this.load(this.page); this.busy = false; }
    },
  },
};
</script>
