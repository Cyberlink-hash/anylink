<template>
  <el-dialog :title="'设备管理 · ' + user.username" :visible="visible" @close="$emit('close')" width="90%" :close-on-click-modal="false" :show-close="!busy">
    <div v-loading="loading">
      <el-alert type="info" :closable="false" title="已批准设备仍需密码和 OTP 认证。设备标识由客户端上报，不代表硬件认证；未提供标识的 OpenConnect 等客户端在开启绑定后无法连接。"></el-alert>
      <div class="device-actions">
        <el-tag :type="policy.enabled ? 'success' : 'info'">设备绑定：{{ policy.enabled ? '已开启' : '未开启' }}</el-tag>
        <span>已批准 {{ approvedDevices.length }} 台 · 待审批 {{ pendingCount }} 台</span>
        <el-button size="small" @click="load" :disabled="busy">刷新</el-button>
        <el-button size="small" @click="change('import')" :disabled="busy">导入近期已连接设备</el-button>
        <el-button v-if="!policy.enabled" size="small" type="primary" @click="enable" :disabled="busy || (!selected.length && !approvedDevices.length)">批准所选设备并开启绑定</el-button>
        <el-button v-else size="small" type="warning" @click="disable" :disabled="busy">关闭绑定</el-button>
      </div>
      <p class="device-help">导入仅取最近30天、最近500条连接成功日志，最多保留50台设备；历史记录只作为候选，需核实后批准。首次登记优先绑定 unique-id-global，缺失时绑定 unique-id，之后固定匹配该字段。开启绑定会断开未批准设备，已批准设备继续在线。</p>
      <el-table ref="devices" :data="devices" row-key="id" border max-height="450" @selection-change="selected = $event" :empty-text="'尚无设备记录，可导入历史记录或让用户正常登录一次'">
        <el-table-column v-if="!policy.enabled" type="selection" width="45" :selectable="selectable"></el-table-column>
        <el-table-column prop="id" label="设备 / 申请号" width="100"></el-table-column>
        <el-table-column label="设备备注" min-width="140">
          <template slot-scope="{ row }">
            <div>{{ row.label || '未备注' }}</div>
            <el-button type="text" size="mini" @click="rename(row)" :disabled="busy">修改备注</el-button>
          </template>
        </el-table-column>
        <el-table-column label="平台 / 客户端" min-width="170">
          <template slot-scope="{ row }">
            <div>{{ row.device_type || '未知设备' }} {{ row.platform_version }}</div>
            <div class="device-help">{{ row.user_agent || '历史日志未保存完整客户端信息' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="绑定标识" min-width="180">
          <template slot-scope="{ row }">
            <div>{{ row.key_kind }}</div>
            <el-popover placement="top" width="460" trigger="click">
              <div class="device-hash">{{ row.key_hash }}</div>
              <el-button slot="reference" type="text" size="mini">{{ row.key_hash.slice(0, 16) }}…</el-button>
            </el-popover>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template slot-scope="{ row }"><el-tag size="small" :type="stateType(row.state)">{{ stateLabel(row.state) }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="remote_addr" label="最近来源 IP" min-width="130"></el-table-column>
        <el-table-column prop="last_seen" label="最近认证 / 连接" :formatter="tableDateFormat" width="160"></el-table-column>
        <el-table-column label="操作" min-width="200">
          <template slot-scope="{ row }">
            <el-button v-if="row.state !== 'approved'" type="text" @click="approve(row)" :disabled="busy">批准新增</el-button>
            <el-button v-if="row.state !== 'approved' && approvedDevices.length" type="text" @click="openReplace(row)" :disabled="busy">替换旧设备</el-button>
            <el-button v-if="row.state === 'observed' || row.state === 'pending'" type="text" @click="confirmChange('reject', row, '拒绝此设备申请？')" :disabled="busy">拒绝</el-button>
            <el-button v-if="row.state !== 'approved'" type="text" @click="confirmChange('delete', row, '删除此记录？该设备再次完成认证后可以重新申请，但不会自动获批。')" :disabled="busy">删除记录</el-button>
            <el-button v-if="row.state === 'approved'" type="text" @click="confirmChange('revoke', row, '撤销此设备授权并断开相关连接？')" :disabled="busy">撤销</el-button>
          </template>
        </el-table-column>
      </el-table>
      <p class="device-help">“候选”不代表已授权；“最近认证 / 连接”来自完整认证后的上报或历史成功连接。拒绝、撤销的设备不会因再次登录自动恢复授权。关闭绑定后不再限制设备。</p>
      <el-collapse>
        <el-collapse-item title="最近50条设备管理记录" name="audit">
          <el-table :data="audits" border max-height="260">
            <el-table-column prop="created_at" label="时间" :formatter="tableDateFormat" width="170"></el-table-column>
            <el-table-column prop="actor" label="管理员" width="120"></el-table-column>
            <el-table-column prop="device_id" label="设备号" width="100"></el-table-column>
            <el-table-column label="操作"><template slot-scope="{ row }">{{ actionLabel(row.action) }}</template></el-table-column>
            <el-table-column prop="detail" label="备注"></el-table-column>
          </el-table>
        </el-collapse-item>
      </el-collapse>
    </div>
    <el-dialog title="替换已批准的旧设备" :visible.sync="replaceVisible" append-to-body width="450px" :close-on-click-modal="false">
      <p>新设备 #{{ replacement.device_id }} 批准后，所选旧设备将被撤销并断开连接。</p>
      <el-select v-model="replacement.replace_id" placeholder="选择旧设备" style="width: 100%">
        <el-option v-for="device in approvedDevices" :key="device.id" :value="device.id" :label="'#' + device.id + ' ' + (device.label || device.device_type || '未备注设备')"></el-option>
      </el-select>
      <span slot="footer"><el-button @click="replaceVisible = false" :disabled="busy">取消</el-button><el-button type="primary" @click="replace" :disabled="!replacement.replace_id || busy">确认替换</el-button></span>
    </el-dialog>
  </el-dialog>
</template>

<script>
import axios from 'axios';

export default {
  name: 'UserDevices',
  props: { user: { type: Object, required: true }, visible: { type: Boolean, default: false } },
  data() {
    return { loading: false, busy: false, policy: { enabled: false, revision: 0 }, devices: [], audits: [], selected: [], replaceVisible: false, replacement: { device_id: 0, replace_id: 0 } };
  },
  computed: {
    approvedDevices() { return this.devices.filter(row => row.state === 'approved'); },
    pendingCount() { return this.devices.filter(row => row.state === 'pending').length; },
  },
  mounted() { this.load(); },
  methods: {
    selectable(row) { return !this.busy && ['observed', 'pending'].includes(row.state); },
    stateLabel(state) { return { observed: '候选', pending: '待审批', approved: '已批准', rejected: '已拒绝', revoked: '已撤销' }[state] || state; },
    stateType(state) { return { observed: 'info', pending: 'warning', approved: 'success', rejected: 'danger', revoked: 'danger' }[state] || 'info'; },
    actionLabel(action) { return { enable: '开启绑定', disable: '关闭绑定', approve: '批准新增', replace: '替换设备', reject: '拒绝申请', revoke: '撤销授权', rename: '修改备注', delete: '删除记录', import: '导入候选', approved: '设备已批准', rejected: '设备已拒绝', revoked: '设备已撤销' }[action] || action; },
    async load() {
      this.loading = true;
      try {
        const { data } = await axios.get('/user/devices', { params: { user_id: this.user.id } });
        if (data.code !== 0) { throw new Error(data.msg); }
        this.policy = data.data.policy;
        this.devices = data.data.devices || [];
        this.audits = data.data.audits || [];
        this.selected = [];
        if (this.$refs.devices) { this.$refs.devices.clearSelection(); }
      } catch (error) { this.$message.error(error.message || '设备列表加载失败'); }
      finally { this.loading = false; }
    },
    async change(action, fields = {}) {
      if (this.busy || this.loading) { return; }
      this.busy = true;
      try {
        const { data } = await axios.post('/user/devices/change', { user_id: this.user.id, revision: this.policy.revision, action, ...fields });
        if (data.code !== 0) { throw new Error(data.msg); }
        this.$message.success('操作成功，已断开 ' + data.data.closed_sessions + ' 条连接');
        this.replaceVisible = false;
        this.$emit('changed');
        window.dispatchEvent(new Event('user-devices-changed'));
      } catch (error) { this.$message.error(error.message || '操作失败，请刷新确认'); }
      finally { await this.load(); this.busy = false; }
    },
    enable() {
      const ids = this.selected.map(row => row.id);
      this.$confirm('确认已核实所选设备？开启后只允许已批准设备连接，未批准设备及无标识客户端的现有连接将被断开。', '开启设备绑定').then(() => this.change('enable', { device_ids: ids })).catch(() => {});
    },
    disable() { this.$confirm('关闭后该用户可使用任意设备登录，仍需通过原有身份认证。是否继续？', '关闭设备绑定').then(() => this.change('disable')).catch(() => {}); },
    approve(row) { this.confirmChange('approve', row, '确认已通过可信方式核实设备 #' + row.id + ' 属于该用户，并批准新增？'); },
    confirmChange(action, row, message) { this.$confirm(message, '设备管理').then(() => this.change(action, { device_id: row.id })).catch(() => {}); },
    rename(row) { this.$prompt('输入便于识别的备注，例如“张三的工作电脑”', '设备备注', { inputValue: row.label, inputValidator: value => (!value || value.length <= 128) || '最多128个字符' }).then(({ value }) => this.change('rename', { device_id: row.id, label: value || '' })).catch(() => {}); },
    openReplace(row) { this.replacement = { device_id: row.id, replace_id: 0 }; this.replaceVisible = true; },
    replace() { this.change('replace', this.replacement); },
  },
};
</script>

<style scoped>
.device-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin: 16px 0; }
.device-help { color: #88909B; font-size: 12px; line-height: 1.6; }
.device-hash { font-family: monospace; word-break: break-all; user-select: all; }
</style>
