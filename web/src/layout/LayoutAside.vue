<template>

  <!--background-color="#304156"-->
  <!--text-color="#bfcbd9"-->
  <!--active-text-color="#409EFF"-->
  <!--:unique-opened="false"-->

  <!--<div class="layout-aside" :style="aside_style">-->

  <el-menu :collapse="!is_active" :default-active="route_path" :style="is_active ? 'width:200px' : ''" router
    class="layout-menu" :collapse-transition="false" background-color="#545c64" text-color="#fff"
    active-text-color="#ffd04b">
    <el-menu-item index="/admin/home">
      <i class="el-icon-s-home"></i>
      <span slot="title">首页</span>
    </el-menu-item>

    <el-submenu index="1">
      <template slot="title">
        <i class="el-icon-menu"></i>
        <span slot="title">基础信息</span>
      </template>

      <el-menu-item index="/admin/set/system">系统信息</el-menu-item>
      <el-menu-item index="/admin/set/soft">软件配置</el-menu-item>
      <el-menu-item index="/admin/set/other">其他设置</el-menu-item>
      <el-menu-item index="/admin/set/audit">审计日志</el-menu-item>
    </el-submenu>

    <el-submenu index="2">
      <template slot="title">
        <i class="el-icon-s-custom"></i>
        <span slot="title">用户信息</span>
      </template>

      <el-menu-item index="/admin/user/list">用户列表</el-menu-item>
      <el-menu-item index="/admin/user/device-reviews">设备审批 <el-badge :value="pendingDevices" :max="99" :hidden="pendingDevices === 0" /></el-menu-item>
      <el-menu-item index="/admin/user/policy">用户连接策略</el-menu-item>
      <el-menu-item index="/admin/user/online">在线用户</el-menu-item>
      <el-menu-item index="/admin/user/lockmanager">锁定管理</el-menu-item>
      <el-menu-item index="/admin/user/ip_map">IP映射</el-menu-item>
    </el-submenu>

    <el-submenu index="3">
      <template slot="title">
        <i class="el-icon-s-order"></i>
        <span slot="title">用户组信息</span>
      </template>

      <el-menu-item index="/admin/group/list">用户组列表</el-menu-item>
      <el-menu-item index="/admin/network_policy/list">网络策略组</el-menu-item>
    </el-submenu>

    <el-submenu index="4">
      <template slot="title">
        <i class="el-icon-s-order"></i>
        <span slot="title">调试信息</span>
      </template>

      <el-menu-item>
        <a href="/debug/pprof/" target="_blank">pprof</a>
      </el-menu-item>
      <el-menu-item>
        <a href="/debug/statsviz/" target="_blank">statsviz</a>
      </el-menu-item>

    </el-submenu>

  </el-menu>

</template>

<script>
import axios from "axios";
export default {
  name: "LayoutAside",
  data() {
    return { pendingDevices: 0, pendingLoading: false, pendingTimer: null }
  },
  props: ['is_active', 'route_path'],
  mounted() {
    this.refreshPending();
    this.pendingTimer = window.setInterval(this.refreshPending, 60000);
    window.addEventListener('user-devices-changed', this.refreshPending);
  },
  beforeDestroy() {
    window.clearInterval(this.pendingTimer);
    window.removeEventListener('user-devices-changed', this.refreshPending);
  },
  watch: { route_path() { this.refreshPending(); } },
  methods: {
    async refreshPending() {
      if (this.pendingLoading) { return; }
      this.pendingLoading = true;
      try {
        const { data } = await axios.get('/user/device-pending-count');
        if (data.code === 0) { this.pendingDevices = data.data.count; }
      } catch (_) { /* Keep the last known count on temporary request failures. */ }
      finally { this.pendingLoading = false; }
    },
  }
}
</script>

<style scoped>
.layout-menu {
  height: 100%;
}

.el-menu-item a {
  display: block;
  color: #fff;
}
</style>
