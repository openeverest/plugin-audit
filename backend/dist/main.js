let b = null;
const L = (n, t) => {
  b = n;
}, F = () => {
  b = null;
}, _ = () => b || localStorage.getItem("everestToken");
let m = null;
const O = () => (m || (m = fetch("/v1/auth/token", {
  method: "POST",
  credentials: "include",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    grant_type: "refresh_token",
    refresh_token_delivery: "cookie"
  })
}).then(async (t) => {
  if (!t.ok) throw new Error(`refresh failed: ${t.status}`);
  const a = await t.json();
  return L(a.access_token, a.expires_in), a.access_token;
}).catch(() => (F(), null)).finally(() => {
  m = null;
})), m);
let e, T;
async function j(n) {
  const t = () => {
    const o = {}, s = _();
    return s && (o.Authorization = `Bearer ${s}`), T(n, { credentials: "include", headers: o });
  };
  let a = await t();
  return a.status === 401 && await O() && (a = await t()), a;
}
async function u(n) {
  const t = await j(`/api${n}`);
  if (!t.ok) {
    const a = await t.text().catch(() => "");
    throw new Error(a || `HTTP ${t.status}`);
  }
  return t.json();
}
function D(n) {
  const t = new URLSearchParams();
  n.types.length && t.set("types", n.types.join(",")), n.namespaces.length && t.set("namespaces", n.namespaces.join(",")), n.search.trim() && t.set("search", n.search.trim()), n.since && t.set("since", new Date(n.since).toISOString()), n.until && t.set("until", new Date(n.until).toISOString()), t.set("limit", String(n.limit)), n.beforeID && t.set("beforeID", String(n.beforeID));
  const a = t.toString();
  return a ? `?${a}` : "";
}
const p = {
  types: [],
  namespaces: [],
  search: "",
  since: "",
  until: "",
  limit: 100,
  beforeID: null
}, r = {
  page: { padding: "1.5rem", fontFamily: "system-ui, sans-serif" },
  h1: { margin: "0 0 1rem", fontSize: "1.5rem" },
  bar: {
    display: "flex",
    flexWrap: "wrap",
    gap: "0.75rem",
    alignItems: "flex-end",
    padding: "1rem",
    background: "#f5f7fa",
    borderRadius: 8,
    marginBottom: "1rem"
  },
  field: { display: "flex", flexDirection: "column", gap: 4 },
  label: { fontSize: "0.75rem", color: "#555" },
  input: {
    padding: "0.4rem 0.6rem",
    border: "1px solid #ccc",
    borderRadius: 4,
    fontSize: "0.875rem",
    minWidth: 180
  },
  button: {
    padding: "0.45rem 0.9rem",
    border: "1px solid #1976d2",
    background: "#1976d2",
    color: "#fff",
    borderRadius: 4,
    cursor: "pointer",
    fontSize: "0.875rem"
  },
  buttonGhost: {
    padding: "0.45rem 0.9rem",
    border: "1px solid #ccc",
    background: "#fff",
    color: "#333",
    borderRadius: 4,
    cursor: "pointer",
    fontSize: "0.875rem"
  },
  table: {
    width: "100%",
    borderCollapse: "collapse",
    fontSize: "0.85rem",
    background: "#fff"
  },
  th: {
    textAlign: "left",
    padding: "0.5rem 0.75rem",
    borderBottom: "2px solid #e0e0e0",
    background: "#fafbfc",
    position: "sticky",
    top: 0
  },
  td: { padding: "0.5rem 0.75rem", borderBottom: "1px solid #eee", verticalAlign: "top" },
  rowHover: { cursor: "pointer" },
  pill: {
    display: "inline-block",
    padding: "0.1rem 0.5rem",
    background: "#eef2ff",
    color: "#3730a3",
    borderRadius: 12,
    fontSize: "0.75rem",
    fontFamily: "ui-monospace, monospace"
  },
  drawer: {
    position: "fixed",
    top: 0,
    right: 0,
    bottom: 0,
    width: "50%",
    minWidth: 480,
    background: "#fff",
    boxShadow: "-8px 0 24px rgba(0,0,0,0.12)",
    padding: "1.5rem",
    overflow: "auto",
    zIndex: 1e3
  },
  drawerClose: {
    position: "absolute",
    top: 12,
    right: 16,
    border: "none",
    background: "transparent",
    fontSize: "1.5rem",
    cursor: "pointer"
  },
  pre: {
    background: "#0d1117",
    color: "#c9d1d9",
    padding: "1rem",
    borderRadius: 6,
    overflow: "auto",
    fontSize: "0.75rem",
    fontFamily: "ui-monospace, monospace"
  },
  banner: (n) => ({
    padding: "0.5rem 0.75rem",
    borderRadius: 4,
    marginBottom: "0.75rem",
    fontSize: "0.85rem",
    background: n === "error" ? "#fdecea" : "#e3f2fd",
    color: n === "error" ? "#b71c1c" : "#0d47a1"
  })
};
function C(n) {
  const t = new Date(n);
  return isNaN(t.getTime()) ? n : t.toLocaleString();
}
const K = (n) => {
  const [t, a] = e.useState(p), [o, s] = e.useState(p), [i, E] = e.useState([]), [y, v] = e.useState(null), [A, $] = e.useState([]), [R, z] = e.useState([]), [g, d] = e.useState(!1), [S, h] = e.useState(null), [k, w] = e.useState(null);
  e.useEffect(() => {
    u("/events/types").then((l) => $(l.items ?? [])).catch(() => {
    }), u("/events/namespaces").then((l) => z(l.items ?? [])).catch(() => {
    });
  }, []), e.useEffect(() => {
    let l = !1;
    return d(!0), h(null), u("/events" + D(t)).then((c) => {
      l || (E(c.items ?? []), v(c.nextBeforeID ?? null));
    }).catch((c) => !l && h(c.message)).finally(() => !l && d(!1)), () => {
      l = !0;
    };
  }, [t]);
  const x = () => a({ ...o, beforeID: null }), N = () => {
    s(p), a(p);
  }, B = () => {
    y != null && (d(!0), u("/events" + D({ ...t, beforeID: y })).then((l) => {
      E((c) => [...c, ...l.items ?? []]), v(l.nextBeforeID ?? null);
    }).catch((l) => h(l.message)).finally(() => d(!1)));
  };
  return e.createElement(
    "div",
    { style: r.page },
    e.createElement("h1", { style: r.h1 }, "📜 Audit Log"),
    // Filter bar
    e.createElement(
      "div",
      { style: r.bar },
      e.createElement(
        "div",
        { style: r.field },
        e.createElement("label", { style: r.label }, "Event types"),
        e.createElement(I, {
          options: A,
          value: o.types,
          onChange: (l) => s({ ...o, types: l }),
          placeholder: "All types"
        })
      ),
      e.createElement(
        "div",
        { style: r.field },
        e.createElement("label", { style: r.label }, "Namespaces"),
        e.createElement(I, {
          options: R,
          value: o.namespaces,
          onChange: (l) => s({ ...o, namespaces: l }),
          placeholder: "All namespaces"
        })
      ),
      e.createElement(
        "div",
        { style: r.field },
        e.createElement("label", { style: r.label }, "Search"),
        e.createElement("input", {
          style: r.input,
          type: "text",
          placeholder: "name or payload…",
          value: o.search,
          onChange: (l) => s({ ...o, search: l.target.value }),
          onKeyDown: (l) => l.key === "Enter" && x()
        })
      ),
      e.createElement(
        "div",
        { style: r.field },
        e.createElement("label", { style: r.label }, "From"),
        e.createElement("input", {
          style: r.input,
          type: "datetime-local",
          value: o.since,
          onChange: (l) => s({ ...o, since: l.target.value })
        })
      ),
      e.createElement(
        "div",
        { style: r.field },
        e.createElement("label", { style: r.label }, "To"),
        e.createElement("input", {
          style: r.input,
          type: "datetime-local",
          value: o.until,
          onChange: (l) => s({ ...o, until: l.target.value })
        })
      ),
      e.createElement("button", { style: r.button, onClick: x }, "Apply"),
      e.createElement("button", { style: r.buttonGhost, onClick: N }, "Reset")
    ),
    S && e.createElement("div", { style: r.banner("error") }, `✗ ${S}`),
    g && i.length === 0 ? e.createElement("div", { style: r.banner("info") }, "Loading…") : i.length === 0 ? e.createElement(
      "div",
      { style: r.banner("info") },
      "No events yet. Once the plugin daemon connects to /v1/events, captured events will appear here."
    ) : null,
    // Event table
    i.length > 0 && e.createElement(
      "table",
      { style: r.table },
      e.createElement(
        "thead",
        null,
        e.createElement(
          "tr",
          null,
          e.createElement("th", { style: r.th }, "Time"),
          e.createElement("th", { style: r.th }, "Type"),
          e.createElement("th", { style: r.th }, "Namespace"),
          e.createElement("th", { style: r.th }, "Resource"),
          e.createElement("th", { style: r.th }, "Actor")
        )
      ),
      e.createElement(
        "tbody",
        null,
        i.map(
          (l) => e.createElement(
            "tr",
            {
              key: l.id,
              style: r.rowHover,
              onClick: () => w(l)
            },
            e.createElement("td", { style: r.td }, C(l.occurredAt)),
            e.createElement(
              "td",
              { style: r.td },
              e.createElement("span", { style: r.pill }, l.type)
            ),
            e.createElement("td", { style: r.td }, l.namespace ?? "—"),
            e.createElement(
              "td",
              { style: r.td },
              l.resourceKind ? `${l.resourceKind}/${l.resourceName ?? ""}` : "—"
            ),
            e.createElement(
              "td",
              { style: r.td },
              l.actorID ? `${l.actorType ?? ""}:${l.actorID}` : "—"
            )
          )
        )
      )
    ),
    y != null && e.createElement(
      "div",
      { style: { marginTop: "1rem", textAlign: "center" } },
      e.createElement(
        "button",
        { style: r.buttonGhost, onClick: B, disabled: g },
        g ? "Loading…" : "Load more"
      )
    ),
    k && e.createElement(P, { event: k, onClose: () => w(null) })
  );
}, P = (n) => {
  const { event: t, onClose: a } = n;
  return e.createElement(
    "div",
    { style: r.drawer },
    e.createElement("button", { style: r.drawerClose, onClick: a, "aria-label": "Close" }, "×"),
    e.createElement("h2", { style: { marginTop: 0 } }, t.type),
    e.createElement("p", { style: { color: "#555", marginTop: 0 } }, C(t.occurredAt)),
    e.createElement(
      "dl",
      { style: { display: "grid", gridTemplateColumns: "auto 1fr", gap: "0.4rem 1rem", fontSize: "0.875rem" } },
      f("Namespace", t.namespace),
      f("Resource", t.resourceKind ? `${t.resourceKind}/${t.resourceName ?? ""}` : void 0),
      f("Actor", t.actorID ? `${t.actorType ?? ""}:${t.actorID}` : void 0),
      f("Resource version", t.resourceVersion)
    ),
    e.createElement("h3", null, "Envelope"),
    e.createElement("pre", { style: r.pre }, JSON.stringify(t.envelope, null, 2))
  );
};
function f(n, t) {
  return t ? [
    e.createElement("dt", { key: `${n}-l`, style: { color: "#666" } }, n),
    e.createElement("dd", { key: `${n}-v`, style: { margin: 0 } }, t)
  ] : null;
}
const I = (n) => e.createElement(
  "select",
  {
    multiple: !0,
    style: { ...r.input, minHeight: 80, minWidth: 220 },
    value: n.value,
    onChange: (t) => {
      const a = Array.from(t.target.selectedOptions, (o) => o.value);
      n.onChange(a);
    }
  },
  n.options.length === 0 ? e.createElement("option", { disabled: !0, value: "" }, n.placeholder ?? "—") : n.options.map((t) => e.createElement("option", { key: t, value: t }, t))
), H = (n) => {
  e = n.React, T = n.fetch.bind(n), n.registerExtension({
    type: "sidebarItem",
    label: "Audit Log"
  }), n.registerExtension({
    type: "route",
    label: "Audit Log",
    component: K
  });
};
export {
  H as default
};
