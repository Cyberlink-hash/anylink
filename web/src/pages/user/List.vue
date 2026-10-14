<template>
  <div>
    <el-card>
      <el-form :inline="true" @submit.native.prevent="handleSearch">
        <el-form-item>
          <el-button size="small" type="primary" icon="el-icon-plus" @click="handleEdit('')">添加
          </el-button>
        </el-form-item>
        <el-form-item>
          <el-dropdown size="small" placement="bottom">
            <el-upload class="uploaduser" action="uploaduser" accept=".xlsx, .xls" :http-request="upLoadUser" :limit="1"
              :show-file-list="false">
              <el-button size="small" icon="el-icon-upload2" type="primary">批量添加</el-button>
            </el-upload>
            <el-dropdown-menu slot="dropdown">
              <el-dropdown-item>
                <el-link style="font-size:12px;" type="success" href="批量添加用户模版.xlsx"><i
                    class="el-icon-download"></i>下载模版
                </el-link>
              </el-dropdown-item>
            </el-dropdown-menu>
          </el-dropdown>
        </el-form-item>
        <el-form-item>
          <el-button size="small" type="primary" icon="el-icon-refresh" @click="openLDAPSync" :loading="syncLoading">同步 LDAP 用户</el-button>
        </el-form-item>
        <el-form-item label="用户名 / 姓名 / 邮箱">
          <el-input size="small" v-model="searchData" placeholder="请输入内容"
            @keydown.enter.native.prevent="searchEnterFun" clearable></el-input>
        </el-form-item>

        <el-form-item>
          <el-button size="small" type="primary" icon="el-icon-search" @click="handleSearch()">搜索
          </el-button>
          <el-button size="small" icon="el-icon-refresh" @click="reset">重置搜索
          </el-button>
        </el-form-item>
        <el-form-item label="用户组"><el-select v-model="filters.group" clearable filterable size="small" placeholder="全部" @change="handleSearch"><el-option v-for="name in grouNames" :key="name" :label="name" :value="name"></el-option></el-select></el-form-item>
        <el-form-item label="类型"><el-select v-model="filters.auth_type" clearable size="small" placeholder="全部" @change="handleSearch"><el-option v-for="type in ['local', 'ldap', 'radius']" :key="type" :label="type" :value="type"></el-option></el-select></el-form-item>
        <el-form-item label="状态"><el-select v-model="filters.status" clearable size="small" placeholder="全部" @change="handleSearch"><el-option label="启用" value="1"></el-option><el-option label="停用" value="0"></el-option><el-option label="过期" value="2"></el-option></el-select></el-form-item>
        <el-form-item label="OTP"><el-select v-model="filters.otp" clearable size="small" placeholder="全部" @change="handleSearch"><el-option label="已开启" value="enabled"></el-option><el-option label="未开启" value="disabled"></el-option></el-select></el-form-item>
      </el-form>

      <div class="user-actions">
        <span>已选 {{ selectedUsers.length }} 人（仅当前页）</span>
        <el-select v-model="batchAction" size="small" placeholder="批量操作" style="width: 150px">
          <el-option v-for="item in batchActions" :key="item.value" :label="item.label" :value="item.value"></el-option>
        </el-select>
        <el-button size="small" :disabled="!selectedUsers.length || !batchAction || loading" :loading="batchLoading" @click="runBatch">执行</el-button>
      </div>
      <el-table ref="multipleTable" :data="tableData" v-loading="loading" @selection-change="selectedUsers = $event" row-key="id" border>
        <el-table-column type="selection" width="45"></el-table-column>

        <el-table-column sortable="true" prop="id" label="ID" width="60">
        </el-table-column>

        <el-table-column prop="username" label="用户名" width="150">
        </el-table-column>

        <el-table-column prop="auth_type" label="类型" width="90">
          <template slot-scope="scope">
            <el-tag size="small" type="warning">{{ scope.row.auth_type || 'local' }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="nickname" label="姓名" width="100">
        </el-table-column>

        <el-table-column prop="email" label="邮箱">
        </el-table-column>
        <el-table-column prop="disable_otp" label="OTP认证" width="130">
          <template slot-scope="scope">
            <el-button v-if="!scope.row.disable_otp" type="text" icon="el-icon-view" @click="getOtpImg(scope.row)">
              已开启 · 查看
            </el-button>
            <el-tag v-else size="small" type="info">未开启</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="groups" label="用户组">
          <template slot-scope="scope">
            <el-row v-for="item in scope.row.groups" :key="item">{{ item }}</el-row>
          </template>
        </el-table-column>

        <el-table-column prop="status" label="状态" width="70">
          <template slot-scope="scope">
            <el-tag v-if="scope.row.status === 1" type="success">可用</el-tag>
            <el-tag v-if="scope.row.status === 0" type="danger">停用</el-tag>
            <el-tag v-if="scope.row.status === 2">过期</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="updated_at" label="更新时间" :formatter="tableDateFormat">
        </el-table-column>

        <el-table-column label="操作" width="210">
          <template slot-scope="scope">
            <el-button size="mini" type="primary" @click="handleEdit(scope.row)">编辑
            </el-button>

            <!--            <el-popconfirm
                            class="m-left-10"
                            @onConfirm="handleClick('reset',scope.row)"
                            title="确定要重置用户密码和密钥吗？">
                          <el-button
                              slot="reference"
                              size="mini"
                              type="warning">重置
                          </el-button>
                        </el-popconfirm>-->

            <el-popconfirm class="m-left-10" @confirm="handleDel(scope.row)" title="确定要删除用户吗？">
              <el-button slot="reference" size="mini" type="danger">删除
              </el-button>
            </el-popconfirm>

          </template>
        </el-table-column>
      </el-table>

      <div class="sh-20"></div>

      <el-pagination background layout="total, prev, pager, next" :pager-count="11" @current-change="pageChange"
        :current-page="page" :page-size="10" :total="count">
      </el-pagination>

    </el-card>

    <el-dialog title="同步 LDAP 用户" :visible.sync="syncDialog" width="600px" :close-on-click-modal="false" :close-on-press-escape="!syncLoading" :show-close="!syncLoading">
      <el-alert type="info" :closable="false" title="新增用户默认开启 OTP 并发送一次邮件。已有用户保留 OTP 和停用设置；不会删除用户或移除组。"></el-alert>
      <el-select v-model="syncGroupIDs" multiple placeholder="选择已有 LDAP 用户组" style="width: 100%; margin-top: 16px" :disabled="syncLoading">
        <el-option v-for="group in ldapGroups" :key="group.id" :label="group.name" :value="group.id"></el-option>
      </el-select>
      <span slot="footer"><el-button @click="syncDialog = false" :disabled="syncLoading">取消</el-button><el-button type="primary" @click="syncLDAP" :loading="syncLoading" :disabled="!syncGroupIDs.length">开始同步</el-button></span>
    </el-dialog>
    <el-dialog title="操作结果" :visible.sync="resultDialog" width="760px">
      <p>{{ resultSummary }}</p>
      <el-table :data="operationResults" max-height="400" border>
        <el-table-column prop="username" label="用户" width="130"></el-table-column>
        <el-table-column prop="group" label="用户组" width="130"></el-table-column>
        <el-table-column prop="result" label="结果" width="100"></el-table-column>
        <el-table-column prop="detail" label="详情"></el-table-column>
      </el-table>
    </el-dialog>
    <el-dialog title="OTP密钥" :visible.sync="otpImgData.visible" width="350px" center>
      <div style="text-align: center">{{ otpImgData.username }} : {{ otpImgData.nickname }}</div>
      <img :src="otpImgData.base64Img" alt="otp-img" />
    </el-dialog>

    <!--新增、修改弹出框-->
    <el-dialog :close-on-click-modal="false" title="用户" :visible="user_edit_dialog" @close="disVisible" width="650px"
      center>

      <el-form :model="ruleForm" :rules="rules" ref="ruleForm" label-width="100px" class="ruleForm">
        <el-form-item label="用户ID" prop="id">
          <el-input v-model="ruleForm.id" disabled></el-input>
        </el-form-item>
        <el-form-item label="用户名" prop="username">
          <el-input v-model="ruleForm.username" :disabled="ruleForm.id > 0"></el-input>
        </el-form-item>
        <el-form-item label="姓名" prop="nickname">
          <el-input v-model="ruleForm.nickname"></el-input>
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="ruleForm.email"></el-input>
        </el-form-item>

        <el-form-item label="PIN码" prop="pin_code" v-if="ruleForm.auth_type !== 'ldap'">
          <el-input v-model="ruleForm.pin_code" placeholder="不填由系统自动生成"></el-input>
        </el-form-item>

        <el-form-item label="过期时间" prop="limittime">
          <el-date-picker v-model="ruleForm.limittime" type="date" size="small" align="center" style="width:130px"
            :picker-options="pickerOptions" placeholder="选择日期">
          </el-date-picker>
        </el-form-item>

        <el-form-item label="开启OTP" prop="disable_otp">
          <el-switch v-model="ruleForm.disable_otp" :active-value="false" :inactive-value="true" active-text="已开启动态码认证">
          </el-switch>
        </el-form-item>

        <el-form-item label="OTP密钥" prop="otp_secret" v-if="!ruleForm.disable_otp">
          <el-input v-model="ruleForm.otp_secret" placeholder="不填由系统自动生成"></el-input>
        </el-form-item>

        <el-form-item label="用户组" prop="groups">
          <el-checkbox-group v-model="ruleForm.groups">
            <el-checkbox v-for="(item) in grouNames" :key="item" :label="item" :name="item"></el-checkbox>
          </el-checkbox-group>
        </el-form-item>

        <el-form-item label="发送邮件" prop="send_email">
          <el-switch v-model="ruleForm.send_email">
          </el-switch>
        </el-form-item>

        <el-form-item label="状态" prop="status">
          <el-radio-group v-model="ruleForm.status">
            <el-radio :label="1" border>启用</el-radio>
            <el-radio :label="0" border>停用</el-radio>
            <el-radio :label="2" border>过期</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="submitForm('ruleForm')">保存</el-button>
          <!--          <el-button @click="resetForm('ruleForm')">重置</el-button>-->
          <el-button @click="disVisible">取消</el-button>
        </el-form-item>
      </el-form>

    </el-dialog>

  </div>
</template>

<script>
import axios from "axios";

export default {
  name: "List",
  components: {},
  mixins: [],
  created() {
    this.$emit('update:route_path', this.$route.path)
    this.$emit('update:route_name', ['用户信息', '用户列表'])
  },
  mounted() {
    this.getGroups();
    this.getData(1)
  },

  data() {
    return {
      page: 1,
      loading: false,
      requestID: 0,
      selectedUsers: [],
      batchAction: '',
      batchLoading: false,
      batchActions: [{value: 'enable', label: '启用'}, {value: 'disable', label: '停用'}, {value: 'otp_enable', label: '开启 OTP'}, {value: 'otp_disable', label: '关闭 OTP'}, {value: 'delete', label: '删除'}],
      filters: {group: '', auth_type: '', status: '', otp: ''},
      syncDialog: false,
      syncLoading: false,
      syncGroupIDs: [],
      ldapGroups: [],
      resultDialog: false,
      resultSummary: '',
      operationResults: [],
      grouNames: [],
      tableData: [],
      count: 10,
      pickerOptions: {
        disabledDate(time) {
          return time.getTime() < Date.now();
        }
      },
      searchData: '',
      otpImgData: { visible: false, username: '', nickname: '', base64Img: '' },
      ruleForm: {
        send_email: true,
        status: 1,
        groups: [],
      },
      rules: {
        username: [
          { required: true, message: '请输入用户名', trigger: 'blur' },
          { max: 50, message: '长度小于 50 个字符', trigger: 'blur' }
        ],
        nickname: [
          { required: true, message: '请输入用户姓名', trigger: 'blur' }
        ],
        email: [
          { required: true, message: '请输入用户邮箱', trigger: 'blur' },
          { type: 'email', message: '请输入正确的邮箱地址', trigger: ['blur', 'change'] }
        ],
        password: [
          { min: 6, message: '长度大于 6 个字符', trigger: 'blur' }
        ],
        pin_code: [
          { min: 6, message: 'PIN码大于 6 个字符', trigger: 'blur' }
        ],
        date1: [
          { type: 'date', required: true, message: '请选择日期', trigger: 'change' }
        ],
        groups: [
          { type: 'array', required: true, message: '请至少选择一个组', trigger: 'change' }
        ],
        status: [
          { required: true }
        ],
      },
    }
  },

  methods: {
    upLoadUser(item) {
      const formData = new FormData();
      formData.append("file", item.file);
      axios.post('/user/uploaduser', formData, {
        headers: {
          'Content-Type': 'multipart/form-data'
        }
      }).then(resp => {
        if (resp.data.code === 0) {
          this.$message.success(resp.data.data);
          this.getData(1);
        } else {
          this.$message.error(resp.data.msg);
          this.getData(1);
        }
        console.log(resp.data);
      })
    },
    getOtpImg(row) {
      // this.base64Img = Buffer.from(data).toString('base64');
      this.otpImgData.visible = true
      axios.get('/user/otp_qr', {
        params: {
          id: row.id,
          b64: '1',
        }
      }).then(resp => {
        var rdata = resp.data;
        // console.log(resp);
        this.otpImgData.username = row.username;
        this.otpImgData.nickname = row.nickname;
        this.otpImgData.base64Img = 'data:image/png;base64,' + rdata
      }).catch(error => {
        this.$message.error('哦，请求出错');
        console.log(error);
      });
    },
    handleDel(row) {
      axios.post('/user/del?id=' + row.id).then(resp => {
        var rdata = resp.data
        if (rdata.code === 0) {
          this.$message.success(rdata.msg);
          this.getData(1);
        } else {
          this.$message.error(rdata.msg);
        }
        console.log(rdata);
      }).catch(error => {
        this.$message.error('哦，请求出错');
        console.log(error);
      });
    },
    handleEdit(row) {
      !this.$refs['ruleForm'] || this.$refs['ruleForm'].resetFields();
      console.log(row)
      this.user_edit_dialog = true
      if (!row) {
        this.ruleForm = {send_email: true, status: 1, groups: [], disable_otp: false, auth_type: 'local'};
        return;
      }

      axios.get('/user/detail', {
        params: {
          id: row.id,
        }
      }).then(resp => {
        var data = resp.data.data
        // 修改默认不发送邮件
        data.send_email = false
        this.ruleForm = data
      }).catch(error => {
        this.$message.error('哦，请求出错');
        console.log(error);
      });
    },
    handleSearch() {
      console.log(this.searchData)
      this.getData(1, this.searchData)
    },
    pageChange(p) {
      this.getData(p)
    },
    getData(page) {
      this.page = page;
      this.loading = true;
      this.selectedUsers = [];
      if (this.$refs.multipleTable) this.$refs.multipleTable.clearSelection();
      const requestID = ++this.requestID;
      axios.get('/user/list', {params: Object.assign({page: page, prefix: this.searchData.trim()}, this.filters)}).then(resp => {
        if (requestID !== this.requestID) return;
        if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return; }
        const data = resp.data.data;
        this.tableData = data.datas || [];
        this.count = data.count;
        if (!this.tableData.length && page > 1) this.getData(page - 1);
      }).catch(() => this.$message.error('用户列表加载失败')).finally(() => {
        if (requestID === this.requestID) this.loading = false;
      });
    },
    getGroups() {
      axios.get('/group/names', {}).then(resp => {
        var data = resp.data.data
        console.log(data.datas);
        this.grouNames = data.datas;
      }).catch(error => {
        this.$message.error('哦，请求出错');
        console.log(error);
      });
    },
    submitForm(formName) {
      this.$refs[formName].validate((valid) => {
        if (!valid) {
          console.log('error submit!!');
          return false;
        }

        // alert('submit!');
        axios.post('/user/set', this.ruleForm).then(resp => {
          var data = resp.data
          if (data.code === 0) {
            this.$message.success(data.msg);
            this.getData(1);
            this.user_edit_dialog = false
          } else {
            this.$message.error(data.msg);
          }
          console.log(data);
        }).catch(error => {
          this.$message.error('哦，请求出错');
          console.log(error);
        });
      });
    },
    resetForm(formName) {
      this.$refs[formName].resetFields();
    },
    searchEnterFun(e) {
      if (!e.isComposing && e.keyCode !== 229) this.handleSearch();
    },
    runBatch() {
      const action = this.batchAction;
      const ids = this.selectedUsers.map(user => user.id);
      const label = this.batchActions.find(item => item.value === action).label;
      const hint = action === 'delete' ? '删除 LDAP 用户仅删除本地记录，后续同步或登录可能重新创建。' : action === 'disable' ? '停用将同时断开在线连接。' : '';
      this.$confirm('确定对选中的 ' + ids.length + ' 位用户执行“' + label + '”？' + hint, '批量操作').then(() => {
        this.batchLoading = true;
        axios.post('/user/batch', {ids, action}).then(resp => {
          if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return; }
          this.operationResults = resp.data.data;
          this.resultSummary = '批量' + label + '完成，详情如下：';
          this.resultDialog = true;
          this.getData(this.page);
        }).catch(() => this.$message.error('批量操作请求失败，请刷新确认结果')).finally(() => { this.batchLoading = false; });
      }).catch(() => {});
    },
    openLDAPSync() {
      axios.get('/user/ldap/groups').then(resp => {
        if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return; }
        this.ldapGroups = resp.data.data || [];
        this.syncGroupIDs = [];
        this.syncDialog = true;
      }).catch(() => this.$message.error('LDAP 用户组加载失败'));
    },
    syncLDAP() {
      this.syncLoading = true;
      axios.post('/user/ldap/sync', {group_ids: this.syncGroupIDs}).then(resp => {
        if (resp.data.code !== 0) { this.$message.error(resp.data.msg); return; }
        const result = resp.data.data;
        this.operationResults = result.results;
        this.resultSummary = '新增 ' + result.created + ' 人，更新 ' + result.updated + ' 人；邮件发送成功 ' + result.mail_sent + ' 封，失败 ' + result.mail_failed + ' 封。失败或跳过详情如下，补发邮件可在用户编辑页操作。';
        this.resultDialog = true;
        this.syncDialog = false;
        this.getData(1);
      }).catch(() => this.$message.error('同步请求失败，请刷新用户列表确认执行结果')).finally(() => { this.syncLoading = false; });
    },
    reset() {
      this.searchData = "";
      this.filters = {group: '', auth_type: '', status: '', otp: ''};
      this.handleSearch();
    },
  },
}
</script>

<style scoped>
.user-actions { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
</style>
