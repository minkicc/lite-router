const STORAGE_KEY = "lite-router.locale";
const LEGACY_STORAGE_KEY = "local-router.locale";

const messages = {
  "zh-CN": {
    "language.label": "语言",
    "tabs.channels": "渠道",
    "tabs.connect": "接入",
    "tabs.usage": "使用记录",
    "router.toggle": "启动或停止代理",
    "router.checking": "检测中…",
    "router.running": "运行中{pid}",
    "router.stopped": "未启动",
    "router.starting": "正在启动…",
    "router.stopping": "正在停止…",
    "router.started": "代理已启动",
    "router.startFailed": "启动失败，请检查 sidecar 是否已编译",
    "router.stoppedToast": "代理已停止",
    "router.notRunning": "代理未在运行",
    "router.restartingForNetwork": "正在重启代理以应用网络设置…",
    "actions.copy": "复制",
    "actions.edit": "编辑",
    "actions.enable": "启用",
    "actions.disable": "停用",
    "actions.delete": "删除",
    "actions.cancel": "取消",
    "actions.save": "保存",
    "actions.close": "关闭",
    "actions.update": "更新",
    "connect.allowLan": "局域网访问",
    "connect.noToken": "无需 Token",
    "connect.lanUrls": "局域网接入地址",
    "connect.noLanAddress": "未检测到局域网地址",
    "columns.name": "名称",
    "columns.status": "状态",
    "columns.requests": "请求数",
    "columns.lastUsed": "最近使用",
    "columns.actions": "操作",
    "columns.platformModel": "平台模型",
    "columns.upstreamModel": "上游模型",
    "columns.channel": "渠道",
    "columns.enabled": "启用",
    "columns.priority": "优先级",
    "columns.group": "分组",
    "columns.latency": "延迟",
    "columns.model": "模型",
    "columns.time": "时间",
    "columns.duration": "耗时",
    "status.healthy": "健康",
    "status.unknown": "未知",
    "status.unhealthy": "异常",
    "status.disabled": "停用",
    "status.success": "成功",
    "status.failed": "失败",
    "tokens.generate": "生成 Token",
    "tokens.title": "Token 名称",
    "tokens.namePlaceholder": "例如 codex-mac",
    "tokens.generated": "已生成 Token，可在下方列表复制",
    "tokens.generateFailed": "生成失败：{error}",
    "mappings.add": "添加模型映射",
    "mappings.edit": "编辑模型映射",
    "mappings.empty": "还没有模型映射。",
    "mappings.section": "模型映射",
    "mappings.platformHint": "Codex 请求时使用的模型名",
    "mappings.upstreamHint": "实际转发给渠道的模型名",
    "mappings.scope": "限定范围（可选）",
    "mappings.supportedChannels": "所有",
    "mappings.channelHint": "不指定时，仅匹配可用模型中包含该上游模型的渠道",
    "mappings.required": "请填写平台模型和上游模型",
    "groups.title": "分组",
    "groups.add": "添加分组",
    "groups.namePlaceholder": "新分组名称",
    "groups.default": "默认",
    "groups.priority": "优先级 {value}",
    "groups.nameRequired": "请输入分组名称",
    "groups.exists": "分组已存在",
    "channels.add": "添加渠道",
    "channels.edit": "编辑渠道",
    "channels.check": "立即体检",
    "channels.checkStarted": "已触发体检",
    "channels.empty": "还没有渠道，点击「添加渠道」开始。",
    "channels.basic": "基本信息",
    "channels.namePlaceholder": "例如 DeepSeek 便宜渠道",
    "channels.models": "模型",
    "channels.availableModels": "可用模型",
    "channels.modelsHint": "每行一个模型名；填 * 表示接受所有模型。点击「更新」自动从 Base URL 拉取。",
    "channels.routing": "路由参数",
    "channels.priorityHint": "数值越大越优先",
    "channels.groupHint": "在渠道页的分组管理中维护",
    "channels.maxRetries": "最大重试次数",
    "channels.maxRetriesHint": "失败后重试该渠道的次数",
    "channels.enable": "启用该渠道",
    "channels.required": "请填写名称和 Base URL",
    "channels.baseUrlRequired": "请先填写 Base URL",
    "channels.fetchingModels": "正在拉取模型…",
    "channels.fetchFailed": "拉取失败：{error}",
    "channels.fetchSuccess": "已拉取 {count} 个模型",
    "channels.fetchError": "拉取模型失败",
    "usage.empty": "还没有使用记录，启动代理并调用后这里会显示。",
    "usage.success": "成功",
    "usage.failed": "失败",
    "usage.promptTokens": "Prompt Tokens",
    "usage.completionTokens": "Completion Tokens",
    "common.enabled": "启用",
    "common.updatedAt": "更新于 {time}",
    "common.routerUnavailable": "本地代理未响应",
    "common.copyEmpty": "没有可复制的内容",
    "common.copied": "已复制",
    "common.copyFailed": "复制失败，请手动选择复制",
    "common.saveFailed": "保存失败：{error}",
    "common.saved": "已保存并生效",
  },
  "en-US": {
    "language.label": "Language",
    "tabs.channels": "Channels",
    "tabs.connect": "Connect",
    "tabs.usage": "Usage",
    "router.toggle": "Start or stop proxy",
    "router.checking": "Checking…",
    "router.running": "Running{pid}",
    "router.stopped": "Stopped",
    "router.starting": "Starting…",
    "router.stopping": "Stopping…",
    "router.started": "Proxy started",
    "router.startFailed": "Start failed. Check that the sidecar was built.",
    "router.stoppedToast": "Proxy stopped",
    "router.notRunning": "Proxy is not running",
    "router.restartingForNetwork": "Restarting proxy to apply network settings…",
    "actions.copy": "Copy",
    "actions.edit": "Edit",
    "actions.enable": "Enable",
    "actions.disable": "Disable",
    "actions.delete": "Delete",
    "actions.cancel": "Cancel",
    "actions.save": "Save",
    "actions.close": "Close",
    "actions.update": "Update",
    "connect.allowLan": "Allow LAN",
    "connect.noToken": "No Token Required",
    "connect.lanUrls": "LAN Endpoints",
    "connect.noLanAddress": "No LAN address detected",
    "columns.name": "Name",
    "columns.status": "Status",
    "columns.requests": "Requests",
    "columns.lastUsed": "Last Used",
    "columns.actions": "Actions",
    "columns.platformModel": "Client Model",
    "columns.upstreamModel": "Upstream Model",
    "columns.channel": "Channel",
    "columns.enabled": "Enabled",
    "columns.priority": "Priority",
    "columns.group": "Group",
    "columns.latency": "Latency",
    "columns.model": "Model",
    "columns.time": "Time",
    "columns.duration": "Duration",
    "status.healthy": "Healthy",
    "status.unknown": "Unknown",
    "status.unhealthy": "Unhealthy",
    "status.disabled": "Disabled",
    "status.success": "Success",
    "status.failed": "Failed",
    "tokens.generate": "Generate Token",
    "tokens.title": "Token Name",
    "tokens.namePlaceholder": "e.g. codex-mac",
    "tokens.generated": "Token generated. Copy it from the list below.",
    "tokens.generateFailed": "Generation failed: {error}",
    "mappings.add": "Add Model Mapping",
    "mappings.edit": "Edit Model Mapping",
    "mappings.empty": "No model mappings yet.",
    "mappings.section": "Model Mapping",
    "mappings.platformHint": "Model name used by Codex requests",
    "mappings.upstreamHint": "Model name sent to the upstream channel",
    "mappings.scope": "Scope (optional)",
    "mappings.supportedChannels": "All",
    "mappings.channelHint": "When unset, only channels listing the upstream model are matched",
    "mappings.required": "Enter both client and upstream model names",
    "groups.title": "Groups",
    "groups.add": "Add Group",
    "groups.namePlaceholder": "New group name",
    "groups.default": "Default",
    "groups.priority": "Priority {value}",
    "groups.nameRequired": "Enter a group name",
    "groups.exists": "This group already exists",
    "channels.add": "Add Channel",
    "channels.edit": "Edit Channel",
    "channels.check": "Check Health",
    "channels.checkStarted": "Health check started",
    "channels.empty": "No channels yet. Select “Add Channel” to begin.",
    "channels.basic": "Basic Information",
    "channels.namePlaceholder": "e.g. Low-cost DeepSeek",
    "channels.models": "Models",
    "channels.availableModels": "Available Models",
    "channels.modelsHint": "One model per line. Use * to accept all models. Select “Update” to fetch models from the Base URL.",
    "channels.routing": "Routing",
    "channels.priorityHint": "Higher values are preferred",
    "channels.groupHint": "Manage groups on the Channels tab",
    "channels.maxRetries": "Maximum Retries",
    "channels.maxRetriesHint": "Retries on this channel after a failure",
    "channels.enable": "Enable this channel",
    "channels.required": "Enter a name and Base URL",
    "channels.baseUrlRequired": "Enter a Base URL first",
    "channels.fetchingModels": "Fetching models…",
    "channels.fetchFailed": "Fetch failed: {error}",
    "channels.fetchSuccess": "Fetched {count} models",
    "channels.fetchError": "Could not fetch models",
    "usage.empty": "No usage records yet. Requests will appear here after the proxy is used.",
    "usage.success": "Success",
    "usage.failed": "Failed",
    "usage.promptTokens": "Prompt Tokens",
    "usage.completionTokens": "Completion Tokens",
    "common.enabled": "Enabled",
    "common.updatedAt": "Updated {time}",
    "common.routerUnavailable": "Local proxy is not responding",
    "common.copyEmpty": "Nothing to copy",
    "common.copied": "Copied",
    "common.copyFailed": "Copy failed. Select and copy the value manually.",
    "common.saveFailed": "Save failed: {error}",
    "common.saved": "Saved and applied",
  },
};

function normalizeLocale(value) {
  return String(value || "").toLowerCase().startsWith("zh") ? "zh-CN" : "en-US";
}

function initialLocale() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY);
    if (saved) return normalizeLocale(saved);
  } catch {
    // Storage may be unavailable in hardened webviews.
  }
  return normalizeLocale(navigator.languages?.[0] || navigator.language || "zh-CN");
}

let currentLocale = initialLocale();

export function getLocale() {
  return currentLocale;
}

export function setLocale(locale) {
  currentLocale = normalizeLocale(locale);
  try {
    localStorage.setItem(STORAGE_KEY, currentLocale);
  } catch {
    // The active session still switches even if persistence is unavailable.
  }
  document.documentElement.lang = currentLocale;
}

export function t(key, params = {}) {
  const template = messages[currentLocale]?.[key] ?? messages["zh-CN"]?.[key] ?? key;
  return template.replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? ""));
}

export function applyTranslations(root = document) {
  document.documentElement.lang = currentLocale;
  root.querySelectorAll("[data-i18n]").forEach((node) => {
    node.textContent = t(node.dataset.i18n);
  });
  root.querySelectorAll("[data-i18n-placeholder]").forEach((node) => {
    node.placeholder = t(node.dataset.i18nPlaceholder);
  });
  root.querySelectorAll("[data-i18n-aria-label]").forEach((node) => {
    node.setAttribute("aria-label", t(node.dataset.i18nAriaLabel));
  });
}
