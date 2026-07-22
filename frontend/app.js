// Painel de testes do Secullum Compliance — JS puro, sem dependências.

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

// --- Estado ---
const state = {
  get baseUrl() {
    return $("#baseUrl").value.replace(/\/+$/, "");
  },
  selectedTenant: null, // { id, name, ... }
};

// Persiste a base URL entre reloads.
const savedUrl = localStorage.getItem("baseUrl");
if (savedUrl) $("#baseUrl").value = savedUrl;
$("#baseUrl").addEventListener("change", () =>
  localStorage.setItem("baseUrl", $("#baseUrl").value)
);

// --- Log ---
function log(kind, msg, data) {
  const el = $("#log");
  const time = new Date().toLocaleTimeString();
  const cls = kind === "ok" ? "log-ok" : kind === "err" ? "log-err" : "";
  let line = `[${time}] ${msg}`;
  if (data !== undefined) line += "\n" + JSON.stringify(data, null, 2);
  const span = document.createElement("span");
  span.className = cls;
  span.textContent = line + "\n\n";
  el.appendChild(span);
  el.scrollTop = el.scrollHeight;
}
$("#btnClearLog").onclick = () => ($("#log").textContent = "");

// --- Cliente HTTP ---
// Devolve { ok, status, data } e registra tudo no log.
async function api(method, path, body) {
  const url = state.baseUrl + path;
  try {
    const opts = { method, headers: {} };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    let data = null;
    try {
      data = await res.json();
    } catch (_) {
      /* resposta sem corpo */
    }
    if (res.ok) {
      log("ok", `${method} ${path} → ${res.status}`, data);
    } else {
      log("err", `${method} ${path} → ${res.status}`, data);
    }
    return { ok: res.ok, status: res.status, data };
  } catch (err) {
    log("err", `${method} ${path} → falha de rede: ${err.message}` +
      "\n(verifique a base URL, se o backend está no ar e se o CORS está liberado)");
    return { ok: false, status: 0, data: null };
  }
}

// --- Health ---
$("#btnHealth").onclick = async () => {
  const { ok } = await api("GET", "/health");
  $("#healthDot").className = "dot " + (ok ? "ok" : "err");
};

// --- Tenants ---
$("#formTenant").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = e.target;
  const body = {
    name: f.name.value,
    secullum_database_id: Number(f.secullum_database_id.value),
    staff_name: f.staff_name.value,
    staff_contact: f.staff_contact.value,
  };
  const { ok } = await api("POST", "/api/v1/tenants", body);
  if (ok) {
    f.reset();
    listTenants();
  }
});

$("#btnListTenants").onclick = listTenants;

async function listTenants() {
  const inc = $("#includeInactive").checked ? "?include_inactive=true" : "";
  const { ok, data } = await api("GET", "/api/v1/tenants" + inc);
  if (!ok) return;
  const tbody = $("#tenantsTable tbody");
  tbody.innerHTML = "";
  (data.tenants || []).forEach((t) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${t.id}</td>
      <td>${escapeHtml(t.name)}</td>
      <td>${t.secullum_database_id}</td>
      <td><span class="badge ${t.active ? "on" : "off"}">${t.active ? "ativo" : "inativo"}</span></td>
      <td><div class="row-actions">
        <button class="ghost small" data-act="select">Abrir</button>
        <button class="ghost small danger" data-act="deactivate">Desativar</button>
      </div></td>`;
    tr.querySelector('[data-act="select"]').onclick = () => selectTenant(t);
    tr.querySelector('[data-act="deactivate"]').onclick = () => deactivateTenant(t.id);
    tbody.appendChild(tr);
  });
}

async function deactivateTenant(id) {
  if (!confirm(`Desativar o tenant #${id}?`)) return;
  const { ok } = await api("PATCH", `/api/v1/tenants/${id}/deactivate`);
  if (ok) listTenants();
}

function selectTenant(t) {
  state.selectedTenant = t;
  $("#selTenantLabel").textContent = `#${t.id} — ${t.name}`;
  $("#detailCard").classList.remove("hidden");
  // Pré-preenche o form de edição.
  $("#formEditTenant").name.value = t.name;
  $("#formEditTenant").secullum_database_id.value = t.secullum_database_id;
  switchTab("staffs");
  listStaffs();
  $("#detailCard").scrollIntoView({ behavior: "smooth" });
}

// --- Abas ---
$$(".tab").forEach((tab) =>
  (tab.onclick = () => switchTab(tab.dataset.tab))
);
function switchTab(name) {
  $$(".tab").forEach((t) => t.classList.toggle("active", t.dataset.tab === name));
  $$(".tabpane").forEach((p) => p.classList.toggle("hidden", p.dataset.pane !== name));
  if (name === "settings") loadSettings();
  if (name === "reports") listReports();
}

// --- Staff ---
$("#formStaff").addEventListener("submit", async (e) => {
  e.preventDefault();
  const t = state.selectedTenant;
  if (!t) return;
  const f = e.target;
  const body = { name: f.name.value, celular: f.celular.value };
  const { ok } = await api("POST", `/api/v1/tenants/${t.id}/staffs`, body);
  if (ok) {
    f.reset();
    listStaffs();
  }
});

async function listStaffs() {
  const t = state.selectedTenant;
  const { ok, data } = await api("GET", `/api/v1/tenants/${t.id}/staffs`);
  if (!ok) return;
  const tbody = $("#staffsTable tbody");
  tbody.innerHTML = "";
  (data.staffs || []).forEach((s) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${s.id}</td>
      <td>${escapeHtml(s.name)}</td>
      <td>${escapeHtml(s.celular)}</td>
      <td><div class="row-actions">
        <button class="ghost small" data-act="edit">Editar</button>
        <button class="ghost small danger" data-act="del">Excluir</button>
      </div></td>`;
    tr.querySelector('[data-act="edit"]').onclick = () => editStaff(s);
    tr.querySelector('[data-act="del"]').onclick = () => deleteStaff(s.id);
    tbody.appendChild(tr);
  });
}

async function editStaff(s) {
  const name = prompt("Nome do responsável:", s.name);
  if (name === null) return;
  const celular = prompt("Celular:", s.celular);
  if (celular === null) return;
  const { ok } = await api("PUT", `/api/v1/staffs/${s.id}`, { name, celular });
  if (ok) listStaffs();
}

async function deleteStaff(id) {
  if (!confirm(`Excluir o responsável #${id}?`)) return;
  const { ok } = await api("DELETE", `/api/v1/staffs/${id}`);
  if (ok) listStaffs();
}

// --- Configurações ---
async function loadSettings() {
  const t = state.selectedTenant;
  const { ok, data } = await api("GET", `/api/v1/tenants/${t.id}/settings`);
  if (!ok || !data.settings) return;
  const s = data.settings;
  const f = $("#formSettings");
  f.almoco.checked = !!s.almoco;
  f.interjornada.checked = !!s.interjornada;
  f.hextras.checked = !!s.hextras;
  f.esquecimento.checked = !!s.esquecimento;
  f.almoco_severity.value = s.almoco_severity || "";
  f.interjornada_severity.value = s.interjornada_severity || "";
  f.esquecimento_severity.value = s.esquecimento_severity || "";
  f.horarios.value = (s.horarios || []).join(", ");
}

$("#formSettings").addEventListener("submit", async (e) => {
  e.preventDefault();
  const t = state.selectedTenant;
  const f = e.target;
  const horarios = f.horarios.value
    .split(",")
    .map((h) => h.trim())
    .filter(Boolean);
  const body = {
    almoco: f.almoco.checked,
    interjornada: f.interjornada.checked,
    hextras: f.hextras.checked,
    esquecimento: f.esquecimento.checked,
    almoco_severity: f.almoco_severity.value,
    interjornada_severity: f.interjornada_severity.value,
    esquecimento_severity: f.esquecimento_severity.value,
    horarios,
  };
  await api("PUT", `/api/v1/tenants/${t.id}/settings`, body);
});

// --- Editar tenant ---
$("#formEditTenant").addEventListener("submit", async (e) => {
  e.preventDefault();
  const t = state.selectedTenant;
  const f = e.target;
  const body = {
    name: f.name.value,
    secullum_database_id: Number(f.secullum_database_id.value),
  };
  const { ok } = await api("PUT", `/api/v1/tenants/${t.id}`, body);
  if (ok) {
    state.selectedTenant = { ...t, ...body };
    $("#selTenantLabel").textContent = `#${t.id} — ${body.name}`;
    listTenants();
  }
});

// --- Relatórios ---
$("#btnListReports").onclick = listReports;

async function listReports() {
  const t = state.selectedTenant;
  const { ok, data } = await api("GET", `/api/v1/tenants/${t.id}/reports`);
  if (!ok) return;
  const box = $("#reportsList");
  box.innerHTML = "";
  const reports = data.reports || [];
  if (reports.length === 0) {
    box.innerHTML = "<p style='color:#64748b'>Nenhum relatório ainda.</p>";
    return;
  }
  reports.forEach((r) => {
    const div = document.createElement("div");
    div.className = "report";
    const incs = (r.inconsistencies || [])
      .map(
        (i) =>
          `<div class="inc"><span class="sev-${i.Severity}">${i.Severity}</span> — <b>${escapeHtml(i.Type)}</b>: ${escapeHtml(i.Description)}</div>`
      )
      .join("");
    div.innerHTML = `<h4>Relatório #${r.id} — ${r.date} (${r.total} inconsistência(s))</h4>${incs}`;
    box.appendChild(div);
  });
}

// --- Disparar auditoria ---
$("#btnTrigger").onclick = async () => {
  const t = state.selectedTenant;
  await api("POST", "/api/v1/audit/trigger", { tenant_id: t.id });
};

// --- util ---
function escapeHtml(str) {
  return String(str ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

// Primeira carga.
log("info", "Painel pronto. Configure a base URL e clique em 'Testar /health'.");
