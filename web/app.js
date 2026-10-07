const $ = (s) => document.querySelector(s);

const CATS = {
  health: ["สุขภาพ / หาหมอ", "🏥"],
  repair: ["ซ่อมแซม", "🔧"],
  gadget: ["เครื่องใช้ / ไอที", "📱"],
  travel: ["เที่ยว", "✈️"],
  gift: ["ของขวัญ / งานสังคม", "🎁"],
  edu: ["การศึกษา", "📚"],
  bill: ["ภาษี / ค่าธรรมเนียม", "🧾"],
  other: ["อื่นๆ", "🐟"],
};

// Size of a catch relative to the usual monthly spending.
const TIERS = [
  { max: 0.05, cls: "tier-1", name: "ปลาทู" },
  { max: 0.15, cls: "tier-2", name: "ปลาแซลมอน" },
  { max: 0.3, cls: "tier-3", name: "ปลาทูน่า" },
  { max: Infinity, cls: "tier-4", name: "ฉลามยักษ์!" },
];

const money = (n) => new Intl.NumberFormat("th-TH", { maximumFractionDigits: 2 }).format(n);
const short = (n) => (n >= 1000 ? `${money(Math.round(n / 100) / 10)}k` : money(n));
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};
const shiftMonth = (m, delta) => {
  const [y, mo] = m.split("-").map(Number);
  const d = new Date(y, mo - 1 + delta, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
};
const monthName = (m, opts = { month: "long", year: "numeric" }) =>
  new Date(`${m}-01T00:00:00`).toLocaleDateString("th-TH", opts);

const tierOf = (amount, baseline) => {
  const ratio = baseline > 0 ? amount / baseline : 1;
  return TIERS.find((t) => ratio < t.max);
};

let month = today().slice(0, 7);
let settings = { baseline: 0, threshold: 0 };
let token = null; // LIFF ID token, or "dev:<name>" in DEV_MODE
let inLiff = false;
let greeting = "";

async function api(path, opts = {}) {
  const res = await fetch(path, {
    ...opts,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
  });
  if (res.status === 401 && inLiff) {
    // ID token expired: log in again to get a fresh one.
    liff.login({ redirectUri: location.href });
    throw new Error("กำลังเข้าสู่ระบบใหม่…");
  }
  const body = res.status === 204 ? null : await res.json();
  if (!res.ok) throw Object.assign(new Error(body?.error || res.statusText), { status: res.status });
  return body;
}

function say(text, worried = false) {
  const b = $("#bubble");
  b.textContent = text;
  b.classList.remove("pop");
  void b.offsetWidth;
  b.classList.add("pop");
  $("#chef").classList.toggle("worried", worried);
}

function toast(text) {
  const t = $("#toast");
  t.textContent = text;
  t.classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => t.classList.remove("show"), 2600);
}

function chefComment(sum) {
  const ratio = sum.settings.baseline > 0 ? sum.total / sum.settings.baseline : 0;
  if (sum.count === 0) return ["ทะเลสงบ~ เดือนนี้ยังไม่มีปลาใหญ่เลย ดีมาก!", false];
  const vs = sum.prevTotal > 0 && sum.total > sum.prevTotal * 1.5 ? " แถมเยอะกว่าเดือนก่อนอีก!" : "";
  if (ratio < 0.1) return [`ได้ปลาเล็กๆ ${sum.count} ตัว ยังสบายๆ${vs}`, false];
  if (ratio < 0.3) return [`เริ่มมีปลาใหญ่ขึ้นสายพานแล้ว ระวังหน่อยนะ${vs}`, !!vs];
  return [`โอ้โห! เดือนนี้จ่ายพิเศษ ${Math.round(ratio * 100)}% ของปกติเลย${vs}`, true];
}

function renderSummary(sum) {
  settings = sum.settings;
  $("#month-label").textContent = monthName(month);
  $("#total").textContent = money(sum.total);
  $("#count").textContent = sum.count;
  $("#biggest").textContent = sum.biggest ? `${sum.biggest.title} ฿${short(sum.biggest.amount)}` : "—";

  const vs = $("#vs-prev");
  vs.className = "";
  if (sum.prevTotal === 0) vs.textContent = sum.total > 0 ? "เดือนก่อนว่าง" : "—";
  else {
    const diff = sum.total - sum.prevTotal;
    vs.textContent = `${diff > 0 ? "▲" : diff < 0 ? "▼" : "="} ฿${short(Math.abs(diff))}`;
    vs.className = diff > 0 ? "up" : diff < 0 ? "down" : "";
  }

  const pct = settings.baseline > 0 ? (sum.total / settings.baseline) * 100 : 0;
  const fill = $("#meter-fill");
  fill.style.width = `${Math.min(pct, 100)}%`;
  fill.className = `meter-fill ${pct >= 30 ? "t4" : pct >= 15 ? "t3" : pct >= 5 ? "t2" : ""}`;
  $("#meter").setAttribute("aria-valuenow", Math.round(Math.min(pct, 100)));
  $("#meter-text").textContent = `${pct.toFixed(0)}% ของค่าใช้จ่ายปกติ (฿${money(settings.baseline)})`;

  const max = Math.max(...sum.history.map((h) => h.total), 1);
  $("#bars").innerHTML = sum.history
    .map(
      (h) => `<div class="bar ${h.month === month ? "current" : ""}">
        <span class="val">${h.total ? short(h.total) : ""}</span>
        <div class="col" style="height:${(h.total / max) * 70}%"></div>
        <span class="lbl">${esc(monthName(h.month, { month: "short" }))}</span>
      </div>`
    )
    .join("");

  const [text, worried] = chefComment(sum);
  say(greeting + text, worried);
  greeting = "";
  $("#amount").placeholder = `≥ ${money(settings.threshold)}`;
}

function renderList(list) {
  const ul = $("#list");
  if (list.length === 0) {
    ul.innerHTML = `<li class="empty">สายพานว่างเปล่า 🍥 ยังไม่มีรายจ่ายพิเศษ</li>`;
    return;
  }
  ul.innerHTML = list
    .map((e, i) => {
      const [label, icon] = CATS[e.category] || CATS.other;
      const tier = tierOf(e.amount, settings.baseline);
      const day = new Date(`${e.date}T00:00:00`).toLocaleDateString("th-TH", { day: "numeric", month: "short" });
      return `<li class="dish ${tier.cls}" style="animation-delay:${i * 60}ms">
        <div class="plate" aria-hidden="true">${icon}</div>
        <div class="dish-info"><b>${esc(e.title)}</b><small>${esc(day)} · ${esc(label)}${e.note ? ` · ${esc(e.note)}` : ""}</small></div>
        <div class="dish-amt">฿${money(e.amount)}<em>${tier.name}</em></div>
        <button class="del" data-id="${esc(e.id)}" aria-label="ลบ ${esc(e.title)}">✕</button>
      </li>`;
    })
    .join("");
}

async function load() {
  try {
    const [sum, list] = await Promise.all([
      api(`/api/summary?month=${month}`),
      api(`/api/expenses?month=${month}`),
    ]);
    renderSummary(sum);
    renderList(list);
  } catch (err) {
    say(`ร้านมีปัญหานิดหน่อย: ${err.message}`, true);
  }
}

function syncDate() {
  $("#date").value = month === today().slice(0, 7) ? today() : `${month}-01`;
}

function amountHint() {
  const v = parseFloat($("#amount").value);
  const hint = $("#amount-hint");
  hint.className = "hint full";
  if (!v) hint.textContent = "";
  else if (v < settings.threshold) {
    hint.textContent = `🐟 ตัวเล็กไป ปล่อยคืนทะเลเถอะ (ต่ำกว่า ฿${money(settings.threshold)})`;
    hint.classList.add("warn");
  } else hint.textContent = `ขนาด: ${tierOf(v, settings.baseline).name}`;
}

$("#category").innerHTML = Object.entries(CATS)
  .map(([k, [label, icon]]) => `<option value="${k}">${icon} ${label}</option>`)
  .join("");

$("#prev").onclick = () => { month = shiftMonth(month, -1); syncDate(); load(); };
$("#next").onclick = () => { month = shiftMonth(month, 1); syncDate(); load(); };
$("#amount").addEventListener("input", amountHint);

$("#add-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const f = new FormData(ev.target);
  const body = Object.fromEntries(f);
  body.amount = parseFloat(body.amount);
  try {
    const e = await api("/api/expenses", { method: "POST", body: JSON.stringify(body) });
    ev.target.reset();
    month = e.date.slice(0, 7);
    syncDate();
    amountHint();
    toast(`เสิร์ฟ "${e.title}" ลงสายพานแล้ว 🍣`);
    await load();
  } catch (err) {
    if (err.status === 422) say(`ปลาตัวนี้เล็กไป ปล่อยคืนทะเลนะ — จดเฉพาะตั้งแต่ ฿${money(settings.threshold)}`, true);
    else say(`เสิร์ฟไม่ได้: ${err.message}`, true);
  }
});

$("#list").addEventListener("click", async (ev) => {
  const btn = ev.target.closest(".del");
  if (!btn || !confirm("เอาจานนี้ออกจากสายพาน?")) return;
  try {
    await api(`/api/expenses/${btn.dataset.id}`, { method: "DELETE" });
    toast("เก็บจานแล้ว");
    await load();
  } catch (err) {
    say(`ลบไม่ได้: ${err.message}`, true);
  }
});

const dlg = $("#settings");
$("#open-settings").onclick = () => {
  const f = $("#settings-form");
  f.baseline.value = settings.baseline;
  f.threshold.value = settings.threshold;
  dlg.returnValue = "";
  dlg.showModal();
};
$("#wipe").onclick = async () => {
  if (!confirm("ลบรายจ่ายและการตั้งค่าทั้งหมดของคุณถาวร? กู้คืนไม่ได้นะ")) return;
  dlg.close();
  try {
    await api("/api/me", { method: "DELETE" });
    toast("ล้างร้านเรียบร้อย ข้อมูลทั้งหมดถูกลบแล้ว");
    await load();
  } catch (err) {
    say(`ลบไม่ได้: ${err.message}`, true);
  }
};
dlg.addEventListener("close", async () => {
  if (dlg.returnValue !== "save") return;
  const f = $("#settings-form");
  try {
    await api("/api/settings", {
      method: "PUT",
      body: JSON.stringify({ baseline: +f.baseline.value, threshold: +f.threshold.value }),
    });
    toast("บันทึกการตั้งค่าแล้ว");
    await load();
  } catch (err) {
    say(`บันทึกไม่ได้: ${err.message}`, true);
  }
});

async function boot() {
  try {
    const cfg = await fetch("/api/config").then((r) => r.json());
    let name = "";
    if (cfg.liffId) {
      await liff.init({ liffId: cfg.liffId });
      if (!liff.isLoggedIn()) {
        liff.login({ redirectUri: location.href });
        return;
      }
      inLiff = true;
      token = liff.getIDToken();
      name = await liff.getProfile().then((p) => p.displayName, () => "");
    } else if (cfg.devMode) {
      // ?user=alice simulates a different LINE user.
      name = new URLSearchParams(location.search).get("user") || "local";
      token = `dev:${encodeURIComponent(name)}`;
    } else {
      throw new Error("ยังไม่ได้ตั้งค่า LIFF_ID");
    }
    if (name) greeting = `สวัสดีคุณ${name}! `;
  } catch (err) {
    say(`เปิดร้านไม่ได้: ${err.message}`, true);
    return;
  }
  syncDate();
  load();
}

boot();
