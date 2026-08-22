import { applyTranslations, getLocale, setLocale, t } from "/i18n.js";

const tauri = window.__TAURI__;
const invoke = tauri?.core?.invoke;

const els = {};

let baseUrl = "http://127.0.0.1:8787";
let config = { channels: [], model_mappings: [], health_check: {}, listen_addr: "127.0.0.1:8787" };
let channelsState = [];
let tokens = [];
let modalType = null;
let modalId = null;
let lanAddrs = [];
let routerSwitchBusy = false;
let usageEventSource = null;
let usageRefreshTimer = null;
let usageLoading = false;
let lastProcessStatus = null;
let lastUsageData = null;

function statusLabel(status) {
  const key = ["healthy", "unknown", "unhealthy", "disabled"].includes(status) ? status : "unknown";
  return t(`status.${key}`);
}

function esc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function toast(message) {
  els.toast.textContent = message;
  els.toast.classList.add("show");
  clearTimeout(toast._timer);
  toast._timer = setTimeout(() => els.toast.classList.remove("show"), 2400);
}

async function copyText(text) {
  if (!text) {
    toast(t("common.copyEmpty"));
    return;
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
    toast(t("common.copied"));
  } catch {
    toast(t("common.copyFailed"));
  }
}

async function safeInvoke(cmd, args) {
  if (!invoke) return null;
  try {
    return await invoke(cmd, args);
  } catch {
    return null;
  }
}

function renderProcess(status) {
  lastProcessStatus = status;
  if (!status) {
    els.routerToggle.checked = false;
    els.routerToggle.disabled = routerSwitchBusy;
    els.processState.textContent = t("router.checking");
    return;
  }
  const running = Boolean(status?.running);
  els.routerToggle.checked = running;
  els.routerToggle.disabled = routerSwitchBusy;
  els.processState.textContent = running
    ? t("router.running", { pid: status.pid ? ` (PID ${status.pid})` : "" })
    : t("router.stopped");
}

function renderConnectPanel() {
  els.baseUrl.textContent = `${baseUrl}/v1`;
  renderAuthToggle();
  renderAllowLan();
}

function renderAuthToggle() {
  const noAuth = Boolean(config.no_auth);
  els.noAuthToggle.checked = noAuth;
  els.tokenModule.hidden = noAuth;
}

function renderAllowLan() {
  const on = Boolean(config.allow_lan);
  els.allowLanToggle.checked = on;
  els.lanBlock.hidden = !on;
  if (on) renderLanUrls();
}

function renderLanUrls() {
  const port = portOf(baseUrl);
  const urls = (lanAddrs || []).map((ip) => `http://${ip}:${port}/v1`);
  els.lanUrls.innerHTML = urls.length
    ? urls
        .map(
          (u) => `
          <div class="kv">
            <code>${esc(u)}</code>
            <button class="ghost" data-copy-url="${esc(u)}">${esc(t("actions.copy"))}</button>
          </div>`,
        )
        .join("")
    : `<span class="muted">${esc(t("connect.noLanAddress"))}</span>`;
}

function channelRowHtml(entry, stateMap) {
  const ch = entry.ch;
  const index = entry.index;
  const st = stateMap[ch.id] || {};
  const label = statusLabel(st.status);
  const models = (ch.models || []).map(esc).join(", ");
  const latency = st.response_time_ms > 0 ? `${st.response_time_ms} ms` : "—";
  const enabled = ch.enabled !== false;
  return `
    <tr>
      <td><span class="badge ${esc(st.status || "unknown")}">${esc(label)}</span></td>
      <td>${esc(ch.name || ch.id)}<div class="sub">${esc(ch.id || "")}</div></td>
      <td class="mono">${esc(ch.base_url)}</td>
      <td>${ch.priority ?? 0}</td>
      <td>${esc(ch.group || "default")}</td>
      <td>${latency}</td>
      <td class="mono">${models || "—"}</td>
      <td class="actions-cell">
        <button class="mini" data-action="edit-channel" data-index="${index}">${esc(t("actions.edit"))}</button>
        <button class="mini" data-action="toggle-channel" data-index="${index}">${esc(t(enabled ? "actions.disable" : "actions.enable"))}</button>
        <button class="mini danger" data-action="delete-channel" data-index="${index}">${esc(t("actions.delete"))}</button>
      </td>
    </tr>`;
}

function renderChannels() {
  const stateMap = {};
  (channelsState || []).forEach((c) => {
    stateMap[c.id] = c;
  });

  const groups = config.groups && config.groups.length ? config.groups : [{ name: "default", priority: 0 }];
  const groupByName = Object.fromEntries(groups.map((g) => [g.name, g]));
  const buckets = {};
  (config.channels || []).forEach((ch, index) => {
    let g = ch.group || "default";
    if (!groupByName[g]) g = "default";
    (buckets[g] ||= []).push({ ch, index });
  });

  const ordered = [...groups].sort(
    (a, b) => (b.priority ?? 0) - (a.priority ?? 0) || a.name.localeCompare(b.name),
  );

  let html = "";
  for (const g of ordered) {
    const list = buckets[g.name] || [];
    if (!list.length) continue;
    html += `
      <div class="group-block">
        <div class="group-block-title">${esc(g.name)} <span class="muted">${esc(t("groups.priority", { value: g.priority ?? 0 }))}</span></div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>${esc(t("columns.status"))}</th>
                <th>${esc(t("columns.channel"))}</th>
                <th>Base URL</th>
                <th>${esc(t("columns.priority"))}</th>
                <th>${esc(t("columns.group"))}</th>
                <th>${esc(t("columns.latency"))}</th>
                <th>${esc(t("columns.model"))}</th>
                <th>${esc(t("columns.actions"))}</th>
              </tr>
            </thead>
            <tbody>${list.map((entry) => channelRowHtml(entry, stateMap)).join("")}</tbody>
          </table>
        </div>
      </div>`;
  }

  els.channelGroups.innerHTML = html;
  els.channelEmpty.hidden = (config.channels || []).length > 0;
}

function renderMappings() {
  const rows = (config.model_mappings || [])
    .map((m, index) => {
      const enabled = m.enabled !== false;
      return `
        <tr>
          <td class="mono">${esc(m.platform_model)}</td>
          <td class="mono">${esc(m.upstream_model)}</td>
          <td>${esc(mappingChannelLabel(m.channel_id))}</td>
          <td><span class="badge ${enabled ? "healthy" : "disabled"}">${esc(t(enabled ? "common.enabled" : "status.disabled"))}</span></td>
          <td class="actions-cell">
            <button class="mini" data-action="edit-mapping" data-index="${index}">${esc(t("actions.edit"))}</button>
            <button class="mini" data-action="toggle-mapping" data-index="${index}">${esc(t(enabled ? "actions.disable" : "actions.enable"))}</button>
            <button class="mini danger" data-action="delete-mapping" data-index="${index}">${esc(t("actions.delete"))}</button>
          </td>
        </tr>`;
    })
    .join("");
  els.mappingRows.innerHTML = rows;
  els.mappingEmpty.hidden = (config.model_mappings || []).length > 0;
}

function maskToken(token) {
  if (!token) return "—";
  if (token.length <= 12) return token;
  return `${token.slice(0, 6)}…${token.slice(-6)}`;
}

function formatTime(ts) {
  if (!ts) return "—";
  return new Date(ts * 1000).toLocaleString(getLocale());
}

function formatCompactNumber(value) {
  const number = Number(value) || 0;
  const units = [
    [1e12, "T"],
    [1e9, "G"],
    [1e6, "M"],
    [1e3, "K"],
  ];
  for (const [divisor, suffix] of units) {
    if (Math.abs(number) < divisor) continue;
    const scaled = number / divisor;
    const digits = Math.abs(scaled) >= 100 ? 0 : Math.abs(scaled) >= 10 ? 1 : 2;
    return `${scaled.toFixed(digits).replace(/\.0+$|(\.\d*[1-9])0+$/, "$1")}${suffix}`;
  }
  return String(number);
}

function compactNumberHtml(value) {
  const number = Number(value) || 0;
  return `<span title="${esc(number.toLocaleString(getLocale()))}">${esc(formatCompactNumber(number))}</span>`;
}

function durationHtml(value) {
  const milliseconds = Number(value) || 0;
  if (milliseconds <= 0) return "—";
  const seconds = milliseconds / 1000;
  const digits = seconds >= 100 ? 0 : seconds >= 10 ? 1 : 2;
  const display = seconds.toFixed(digits).replace(/\.0+$|(\.\d*[1-9])0+$/, "$1");
  return `<span title="${esc(milliseconds.toLocaleString(getLocale()))} ms">${esc(display)} s</span>`;
}

function portOf(url) {
  try {
    return new URL(url).port || "8787";
  } catch {
    return "8787";
  }
}

function renderTokens() {
  const rows = (tokens || [])
    .map((token) => {
      const enabled = token.enabled !== false;
      return `
        <tr>
          <td>${esc(token.name)}</td>
          <td class="mono">${esc(maskToken(token.token))}</td>
          <td><span class="badge ${enabled ? "healthy" : "disabled"}">${esc(t(enabled ? "common.enabled" : "status.disabled"))}</span></td>
          <td>${token.request_count ?? 0}</td>
          <td>${compactNumberHtml(token.prompt_tokens)}</td>
          <td>${compactNumberHtml(token.completion_tokens)}</td>
          <td>${formatTime(token.last_used_at)}</td>
          <td class="actions-cell">
            <button class="mini" data-action="copy-token" data-id="${esc(token.id)}">${esc(t("actions.copy"))}</button>
            <button class="mini" data-action="toggle-token" data-id="${esc(token.id)}">${esc(t(enabled ? "actions.disable" : "actions.enable"))}</button>
            <button class="mini danger" data-action="delete-token" data-id="${esc(token.id)}">${esc(t("actions.delete"))}</button>
          </td>
        </tr>`;
    })
    .join("");
  els.tokenRows.innerHTML = rows;
}

function renderGroups() {
  const groups = config.groups && config.groups.length ? config.groups : [{ name: "default", priority: 0 }];
  els.groupList.innerHTML = groups
    .map(
      (g, index) => `
        <div class="group-row">
          <span class="group-name">${esc(g.name)}${g.name === "default" ? ` <span class="muted">(${esc(t("groups.default"))})</span>` : ""}</span>
          <label class="group-priority">${esc(t("columns.priority"))}
            <input type="number" data-action="group-priority" data-index="${index}" value="${g.priority ?? 0}" />
          </label>
          ${g.name === "default" ? "" : `<button class="mini danger" data-action="delete-group" data-index="${index}">${esc(t("actions.delete"))}</button>`}
        </div>`,
    )
    .join("");
}

function usageSummaryHtml(s) {
  if (!s) return "";
  const blocks = [
    [t("columns.requests"), s.request_count ?? 0, false],
    [t("usage.success"), s.success_count ?? 0, false],
    [t("usage.failed"), s.fail_count ?? 0, false],
    [t("usage.promptTokens"), s.prompt_tokens ?? 0, true],
    [t("usage.completionTokens"), s.completion_tokens ?? 0, true],
  ];
  return blocks
    .map(
      ([label, value, compact]) =>
        `<div class="stat"><span>${label}</span><strong>${compact ? compactNumberHtml(value) : value}</strong></div>`,
    )
    .join("");
}

function usageRowHtml(r) {
  const model = r.upstream_model || r.model || "—";
  const status = r.success
    ? `<span class="badge healthy">${esc(t("status.success"))}</span>`
    : `<span class="badge unhealthy">${esc(t("status.failed"))}</span>`;
  return `
    <tr>
      <td>${esc(formatTime(r.time))}</td>
      <td class="mono">${esc(model)}</td>
      <td>${esc(r.channel_name || r.channel_id || "—")}</td>
      <td>${esc(r.token_name || "—")}</td>
      <td>${compactNumberHtml(r.prompt_tokens)}</td>
      <td>${compactNumberHtml(r.completion_tokens)}</td>
      <td>${durationHtml(r.elapsed_ms)}</td>
      <td>${status}</td>
    </tr>`;
}

async function loadUsage() {
  if (usageLoading) return;
  usageLoading = true;
  try {
    const res = await fetch(`${baseUrl}/api/usage`);
    const data = await res.json();
    const records = data.records || [];
    lastUsageData = data;
    els.usageSummary.innerHTML = usageSummaryHtml(data.summary);
    els.usageRows.innerHTML = records.map(usageRowHtml).join("");
    els.usageEmpty.hidden = records.length > 0;
  } catch {
    lastUsageData = null;
    els.usageSummary.innerHTML = "";
    els.usageRows.innerHTML = "";
    els.usageEmpty.hidden = false;
  } finally {
    usageLoading = false;
  }
}

function scheduleUsageRefresh() {
  clearTimeout(usageRefreshTimer);
  usageRefreshTimer = setTimeout(loadUsage, 1000);
}

function stopUsageUpdates() {
  clearTimeout(usageRefreshTimer);
  usageRefreshTimer = null;
  usageEventSource?.close();
  usageEventSource = null;
}

function startUsageUpdates() {
  stopUsageUpdates();
  loadUsage();
  usageEventSource = new EventSource(`${baseUrl}/api/usage/events`);
  usageEventSource.addEventListener("usage", scheduleUsageRefresh);
}

function usageTabActive() {
  return document.querySelector('.tab.active')?.dataset.tab === "usage";
}

async function refresh() {
  const processStatus = await safeInvoke("router_status");
  renderProcess(processStatus);

  const resolvedBase = await safeInvoke("get_router_base_url");
  if (resolvedBase) baseUrl = resolvedBase;

  try {
    const [stateRes, configRes, tokenRes] = await Promise.all([
      fetch(`${baseUrl}/api/state`),
      fetch(`${baseUrl}/api/config`),
      fetch(`${baseUrl}/api/tokens`),
    ]);
    if (!stateRes.ok || !configRes.ok || !tokenRes.ok) throw new Error("upstream not ready");

    const state = await stateRes.json();
    const configData = await configRes.json();
    const tokenData = await tokenRes.json();

    config = configData;
    config.channels ||= [];
    config.model_mappings ||= [];
    config.groups ||= [{ name: "default", priority: 0 }];
    channelsState = state.channels || [];
    lanAddrs = state.lan_addrs || [];
    tokens = tokenData.tokens || [];

    renderConnectPanel();
    renderChannels();
    renderMappings();
    renderGroups();
    renderTokens();
    els.lastUpdated.textContent = t("common.updatedAt", { time: new Date().toLocaleTimeString(getLocale()) });
  } catch {
    els.lastUpdated.textContent = t("common.routerUnavailable");
  }
}

async function saveConfig() {
  const res = await fetch(`${baseUrl}/api/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    toast(t("common.saveFailed", { error: data.error || res.status }));
    return false;
  }
  toast(t("common.saved"));
  return true;
}

function closeModal() {
  modalType = null;
  modalId = null;
  els.modal.classList.add("hidden");
}

function groupOptionsHtml(selected) {
  const groups = config.groups && config.groups.length ? config.groups : [{ name: "default", priority: 0 }];
  const current = selected || "default";
  return groups
    .map((g) => `<option value="${esc(g.name)}" ${g.name === current ? "selected" : ""}>${esc(g.name)}</option>`)
    .join("");
}

function channelOptionsHtml(selected) {
  const current = selected || "";
  const options = (config.channels || []).map((channel) => {
    const label = `${channel.name || channel.id} (${channel.id})`;
    return `<option value="${esc(channel.id)}" ${channel.id === current ? "selected" : ""}>${esc(label)}</option>`;
  });
  return `<option value="" ${current ? "" : "selected"}>${esc(t("mappings.supportedChannels"))}</option>${options.join("")}`;
}

function mappingChannelLabel(channelID) {
  if (!channelID) return t("mappings.supportedChannels");
  const channel = (config.channels || []).find((item) => item.id === channelID);
  return channel ? `${channel.name || channel.id} (${channel.id})` : channelID;
}

function channelFormHtml(ch) {
  ch = ch || {};
  return `
    <div class="form-section">
      <div class="form-section-title">${esc(t("channels.basic"))}</div>
      <label>${esc(t("columns.name"))}<input name="name" value="${esc(ch.name || "")}" placeholder="${esc(t("channels.namePlaceholder"))}"></label>
      <label>Base URL *<input name="base_url" value="${esc(ch.base_url || "")}" placeholder="https://api.deepseek.com"></label>
      <label>API Key<input name="api_key" value="${esc(ch.api_key || "")}" placeholder="sk-..."></label>
    </div>
    <div class="form-section">
      <div class="form-section-title">${esc(t("channels.models"))}</div>
      <div class="field">
        <div class="field-head">
          <span class="field-title">${esc(t("channels.availableModels"))}</span>
          <button type="button" class="mini" data-action="fetch-models">${esc(t("actions.update"))}</button>
        </div>
        <textarea name="models" rows="4" placeholder="deepseek-chat">${esc((ch.models || []).join("\n"))}</textarea>
        <small class="field-hint">${esc(t("channels.modelsHint"))}</small>
      </div>
    </div>
    <div class="form-section">
      <div class="form-section-title">${esc(t("channels.routing"))}</div>
      <div class="grid">
        <label>${esc(t("columns.priority"))}<input name="priority" type="number" value="${ch.priority ?? 0}"><small class="field-hint">${esc(t("channels.priorityHint"))}</small></label>
        <label>${esc(t("columns.group"))}<select name="group">${groupOptionsHtml(ch.group)}</select><small class="field-hint">${esc(t("channels.groupHint"))}</small></label>
        <label>${esc(t("channels.maxRetries"))}<input name="max_retries" type="number" value="${ch.max_retries ?? 0}"><small class="field-hint">${esc(t("channels.maxRetriesHint"))}</small></label>
      </div>
    </div>
    <label class="check"><input name="enabled" type="checkbox" ${ch.enabled === false ? "" : "checked"}> ${esc(t("channels.enable"))}</label>`;
}

function mappingFormHtml(m) {
  m = m || {};
  return `
    <div class="form-section">
      <div class="form-section-title">${esc(t("mappings.section"))}</div>
      <label>${esc(t("columns.platformModel"))} *<input name="platform_model" value="${esc(m.platform_model || "")}" placeholder="gpt-5.6-sol"><small class="field-hint">${esc(t("mappings.platformHint"))}</small></label>
      <label>${esc(t("columns.upstreamModel"))} *<input name="upstream_model" value="${esc(m.upstream_model || "")}" placeholder="deepseek-v4-pro"><small class="field-hint">${esc(t("mappings.upstreamHint"))}</small></label>
    </div>
    <div class="form-section">
      <div class="form-section-title">${esc(t("mappings.scope"))}</div>
      <label>${esc(t("columns.channel"))}<select name="channel_id">${channelOptionsHtml(m.channel_id)}</select><small class="field-hint">${esc(t("mappings.channelHint"))}</small></label>
    </div>
    <label class="check"><input name="enabled" type="checkbox" ${m.enabled === false ? "" : "checked"}> ${esc(t("common.enabled"))}</label>`;
}

function tokenFormHtml() {
  return `
    <div class="form-section">
      <label>${esc(t("tokens.title"))}<input name="name" placeholder="${esc(t("tokens.namePlaceholder"))}"></label>
    </div>`;
}

function openChannelForm(index) {
  const ch = index >= 0 ? config.channels[index] : null;
  modalType = "channel";
  modalId = index;
  els.modalTitle.textContent = t(ch ? "channels.edit" : "channels.add");
  els.modalBody.innerHTML = channelFormHtml(ch);
  els.modal.classList.remove("hidden");
}

function openMappingForm(index) {
  const m = index >= 0 ? config.model_mappings[index] : null;
  modalType = "mapping";
  modalId = index;
  els.modalTitle.textContent = t(m ? "mappings.edit" : "mappings.add");
  els.modalBody.innerHTML = mappingFormHtml(m);
  els.modal.classList.remove("hidden");
}

function openTokenForm() {
  modalType = "token";
  els.modalTitle.textContent = t("tokens.generate");
  els.modalBody.innerHTML = tokenFormHtml();
  els.modal.classList.remove("hidden");
}

function collectChannelForm() {
  const f = els.modalBody;
  const val = (name) => (f.querySelector(`[name="${name}"]`)?.value || "").trim();
  const models = val("models")
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  return {
    id: modalId >= 0 ? (config.channels[modalId]?.id || "") : "",
    name: val("name"),
    base_url: val("base_url"),
    api_key: val("api_key"),
    models,
    model_mappings: modalId >= 0 ? (config.channels[modalId]?.model_mappings || {}) : {},
    priority: parseInt(val("priority") || "0", 10),
    group: f.querySelector('[name="group"]').value,
    max_retries: parseInt(val("max_retries") || "0", 10),
    enabled: f.querySelector('[name="enabled"]').checked,
  };
}

function collectMappingForm() {
  const f = els.modalBody;
  return {
    platform_model: f.querySelector('[name="platform_model"]').value.trim(),
    upstream_model: f.querySelector('[name="upstream_model"]').value.trim(),
    channel_id: f.querySelector('[name="channel_id"]').value.trim(),
    enabled: f.querySelector('[name="enabled"]').checked,
  };
}

async function fetchModelsFromForm() {
  const base = (els.modalBody.querySelector('[name="base_url"]')?.value || "").trim();
  const apiKey = (els.modalBody.querySelector('[name="api_key"]')?.value || "").trim();
  const modelsField = els.modalBody.querySelector('[name="models"]');
  if (!base) {
    toast(t("channels.baseUrlRequired"));
    return;
  }
  toast(t("channels.fetchingModels"));
  try {
    const res = await fetch(`${baseUrl}/api/probe_models`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ base_url: base, api_key: apiKey }),
    });
    const data = await res.json();
    if (!res.ok) {
      toast(t("channels.fetchFailed", { error: data.error || res.status }));
      return;
    }
    modelsField.value = (data.models || []).join("\n");
    toast(t("channels.fetchSuccess", { count: (data.models || []).length }));
  } catch {
    toast(t("channels.fetchError"));
  }
}

async function saveModal() {
  if (modalType === "channel") {
    const ch = collectChannelForm();
    if (!ch.name || !ch.base_url) {
      toast(t("channels.required"));
      return;
    }
    if (modalId >= 0) {
      config.channels[modalId] = ch;
    } else {
      config.channels.push(ch);
    }
    if (await saveConfig()) {
      closeModal();
      refresh();
    }
  } else if (modalType === "mapping") {
    const m = collectMappingForm();
    if (!m.platform_model || !m.upstream_model) {
      toast(t("mappings.required"));
      return;
    }
    if (modalId >= 0) {
      config.model_mappings[modalId] = m;
    } else {
      config.model_mappings.push(m);
    }
    if (await saveConfig()) {
      closeModal();
      refresh();
    }
  } else if (modalType === "token") {
    const name = els.modalBody.querySelector('[name="name"]').value.trim() || "token";
    const res = await fetch(`${baseUrl}/api/tokens`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      toast(t("tokens.generateFailed", { error: data.error || res.status }));
      return;
    }
    const data = await res.json();
    void data;
    closeModal();
    refresh();
    toast(t("tokens.generated"));
  }
}

async function addGroup() {
  const name = els.groupInput.value.trim();
  const priority = parseInt(els.groupPriorityInput.value || "0", 10);
  if (!name) {
    toast(t("groups.nameRequired"));
    return;
  }
  if (!config.groups) config.groups = [{ name: "default", priority: 0 }];
  if (config.groups.some((g) => g.name === name)) {
    toast(t("groups.exists"));
    return;
  }
  config.groups.push({ name, priority });
  els.groupInput.value = "";
  els.groupPriorityInput.value = "0";
  if (await saveConfig()) {
    refresh();
  }
}

async function updateGroupPriority(input) {
  const index = Number(input.dataset.index);
  const groups = config.groups || [];
  if (!groups[index]) return;
  groups[index].priority = parseInt(input.value || "0", 10);
  await saveConfig();
}

async function handleAction(action, target) {
  const index = Number(target.dataset.index);
  const id = target.dataset.id;

  if (action === "edit-channel") return openChannelForm(index);
  if (action === "edit-mapping") return openMappingForm(index);
  if (action === "fetch-models") return fetchModelsFromForm();
  if (action === "delete-group") {
    const idx = Number(target.dataset.index);
    const groups = config.groups || [];
    const g = groups[idx];
    if (g && g.name !== "default") {
      config.groups = groups.filter((x) => x.name !== g.name);
      (config.channels || []).forEach((ch) => {
        if (ch.group === g.name) ch.group = "default";
      });
      await saveConfig();
      refresh();
    }
    return;
  }

  if (action === "toggle-channel") {
    config.channels[index].enabled = !(config.channels[index].enabled !== false);
    await saveConfig();
    return refresh();
  }
  if (action === "delete-channel") {
    config.channels.splice(index, 1);
    await saveConfig();
    return refresh();
  }
  if (action === "toggle-mapping") {
    config.model_mappings[index].enabled = !(config.model_mappings[index].enabled !== false);
    await saveConfig();
    return refresh();
  }
  if (action === "delete-mapping") {
    config.model_mappings.splice(index, 1);
    await saveConfig();
    return refresh();
  }

  if (action === "copy-token") {
    const t = tokens.find((x) => x.id === id);
    return copyText(t?.token || "");
  }
  if (action === "toggle-token") {
    const t = tokens.find((x) => x.id === id);
    await fetch(`${baseUrl}/api/tokens/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled: !(t.enabled !== false) }),
    });
    return refresh();
  }
  if (action === "delete-token") {
    await fetch(`${baseUrl}/api/tokens/${encodeURIComponent(id)}`, { method: "DELETE" });
    return refresh();
  }
}

async function startRouter() {
  toast(t("router.starting"));
  const result = await safeInvoke("start_router");
  toast(t(result?.running ? "router.started" : "router.startFailed"));
  setTimeout(refresh, 600);
  return result;
}

async function stopRouter() {
  toast(t("router.stopping"));
  const result = await safeInvoke("stop_router");
  toast(t(result?.running === false ? "router.stoppedToast" : "router.notRunning"));
  setTimeout(refresh, 300);
  return result;
}

async function checkAll() {
  toast(t("channels.checkStarted"));
  await fetch(`${baseUrl}/api/check`, { method: "POST" });
  setTimeout(refresh, 1200);
}

function switchTab(name) {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.classList.toggle("active", tab.dataset.tab === name);
  });
  document.querySelectorAll(".panel").forEach((panel) => {
    panel.classList.toggle("active", panel.id === name);
  });
  if (name === "usage" && !document.hidden) {
    startUsageUpdates();
  } else {
    stopUsageUpdates();
  }
}

function modalDraft() {
  if (els.modal.classList.contains("hidden")) return null;
  if (modalType === "channel") return collectChannelForm();
  if (modalType === "mapping") return collectMappingForm();
  if (modalType === "token") return { name: els.modalBody.querySelector('[name="name"]')?.value || "" };
  return null;
}

function renderLocaleChange(draft) {
  applyTranslations();
  renderProcess(lastProcessStatus);
  renderConnectPanel();
  renderChannels();
  renderMappings();
  renderGroups();
  renderTokens();
  if (lastUsageData) {
    const records = lastUsageData.records || [];
    els.usageSummary.innerHTML = usageSummaryHtml(lastUsageData.summary);
    els.usageRows.innerHTML = records.map(usageRowHtml).join("");
  }
  if (modalType === "channel" && draft) {
    els.modalTitle.textContent = t(modalId >= 0 ? "channels.edit" : "channels.add");
    els.modalBody.innerHTML = channelFormHtml(draft);
  } else if (modalType === "mapping" && draft) {
    els.modalTitle.textContent = t(modalId >= 0 ? "mappings.edit" : "mappings.add");
    els.modalBody.innerHTML = mappingFormHtml(draft);
  } else if (modalType === "token" && draft) {
    els.modalTitle.textContent = t("tokens.generate");
    els.modalBody.innerHTML = tokenFormHtml();
    els.modalBody.querySelector('[name="name"]').value = draft.name;
  }
}

window.addEventListener("DOMContentLoaded", () => {
  els.toast = document.querySelector("#toast");
  els.routerToggle = document.querySelector("#router-toggle");
  els.processState = document.querySelector("#process-state");
  els.baseUrl = document.querySelector("#base-url");
  els.noAuthToggle = document.querySelector("#no-auth-toggle");
  els.tokenModule = document.querySelector("#token-module");
  els.tokenRows = document.querySelector("#token-rows");
  els.channelGroups = document.querySelector("#channel-groups");
  els.channelEmpty = document.querySelector("#channel-empty");
  els.mappingRows = document.querySelector("#mapping-rows");
  els.mappingEmpty = document.querySelector("#mapping-empty");
  els.groupList = document.querySelector("#group-list");
  els.groupInput = document.querySelector("#group-input");
  els.groupPriorityInput = document.querySelector("#group-priority-input");
  els.lastUpdated = document.querySelector("#last-updated");
  els.modal = document.querySelector("#modal");
  els.modalTitle = document.querySelector("#modal-title");
  els.modalBody = document.querySelector("#modal-body");
  els.usageSummary = document.querySelector("#usage-summary");
  els.usageRows = document.querySelector("#usage-rows");
  els.usageEmpty = document.querySelector("#usage-empty");
  els.allowLanToggle = document.querySelector("#allow-lan-toggle");
  els.lanBlock = document.querySelector("#lan-block");
  els.lanUrls = document.querySelector("#lan-urls");
  els.localeSelect = document.querySelector("#locale-select");

  els.localeSelect.value = getLocale();
  applyTranslations();
  void safeInvoke("set_locale", { locale: getLocale() });
  els.localeSelect.addEventListener("change", () => {
    const draft = modalDraft();
    setLocale(els.localeSelect.value);
    void safeInvoke("set_locale", { locale: getLocale() });
    renderLocaleChange(draft);
  });

  els.routerToggle.addEventListener("change", async () => {
    const shouldRun = els.routerToggle.checked;
    routerSwitchBusy = true;
    els.routerToggle.disabled = true;
    els.processState.textContent = t(shouldRun ? "router.starting" : "router.stopping");
    const result = shouldRun ? await startRouter() : await stopRouter();
    routerSwitchBusy = false;
    if (result) {
      renderProcess(result);
    } else {
      await refresh();
    }
  });
  document.querySelector("#btn-check").addEventListener("click", checkAll);
  document.querySelector("#btn-add-channel").addEventListener("click", () => openChannelForm(-1));
  document.querySelector("#btn-add-mapping").addEventListener("click", () => openMappingForm(-1));
  document.querySelector("#btn-add-token").addEventListener("click", openTokenForm);
  document.querySelector("#btn-add-group").addEventListener("click", addGroup);
  document.querySelector("#modal-close").addEventListener("click", closeModal);
  document.querySelector("#modal-cancel").addEventListener("click", closeModal);
  document.querySelector("#modal-save").addEventListener("click", saveModal);

  els.noAuthToggle.addEventListener("change", async () => {
    config.no_auth = els.noAuthToggle.checked;
    els.tokenModule.hidden = config.no_auth;
    await saveConfig();
  });

  els.allowLanToggle.addEventListener("change", async () => {
    config.allow_lan = els.allowLanToggle.checked;
    if (!(await saveConfig())) {
      els.allowLanToggle.checked = !config.allow_lan;
      return;
    }
    toast(t("router.restartingForNetwork"));
    await safeInvoke("stop_router");
    await safeInvoke("start_router");
    setTimeout(refresh, 900);
  });

  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => switchTab(tab.dataset.tab));
  });

  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && usageTabActive()) {
      startUsageUpdates();
    } else {
      stopUsageUpdates();
    }
  });

  document.querySelectorAll("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", () => {
      copyText(btn.dataset.copy === "base-url" ? els.baseUrl.textContent : "");
    });
  });

  document.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-action]");
    if (btn) handleAction(btn.dataset.action, btn);
  });

  document.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-copy-url]");
    if (btn) copyText(btn.dataset.copyUrl);
  });

  document.addEventListener("change", (e) => {
    const input = e.target.closest('[data-action="group-priority"]');
    if (input) updateGroupPriority(input);
  });

  refresh();
  setInterval(refresh, 3000);
});
