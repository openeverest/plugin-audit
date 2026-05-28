import type {
  PluginRegisterFn,
  PluginApi,
  PluginRouteProps,
} from '@openeverest/plugin-sdk';

// React and the host-authenticated fetch are injected by the host at runtime.
let React: PluginApi['React'];
let pluginFetch: PluginApi['fetch'];

// ---------------------------------------------------------------------------
// Types — mirror backend/internal/store.Event
// ---------------------------------------------------------------------------
type AuditEvent = {
  id: number;
  resourceVersion: string;
  type: string;
  occurredAt: string;
  namespace?: string;
  resourceKind?: string;
  resourceName?: string;
  actorType?: string;
  actorID?: string;
  envelope: unknown;
};

type ListResponse = {
  items: AuditEvent[] | null;
  nextBeforeID?: number;
};

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------
async function api<T>(path: string): Promise<T> {
  const res = await pluginFetch(`/api${path}`);
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(text || `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

function buildQuery(f: FilterState): string {
  const p = new URLSearchParams();
  if (f.types.length) p.set('types', f.types.join(','));
  if (f.namespaces.length) p.set('namespaces', f.namespaces.join(','));
  if (f.search.trim()) p.set('search', f.search.trim());
  if (f.since) p.set('since', new Date(f.since).toISOString());
  if (f.until) p.set('until', new Date(f.until).toISOString());
  p.set('limit', String(f.limit));
  if (f.beforeID) p.set('beforeID', String(f.beforeID));
  const s = p.toString();
  return s ? `?${s}` : '';
}

// ---------------------------------------------------------------------------
// Filter state
// ---------------------------------------------------------------------------
type FilterState = {
  types: string[];
  namespaces: string[];
  search: string;
  since: string; // datetime-local string
  until: string;
  limit: number;
  beforeID: number | null;
};

const emptyFilter: FilterState = {
  types: [],
  namespaces: [],
  search: '',
  since: '',
  until: '',
  limit: 100,
  beforeID: null,
};

// ---------------------------------------------------------------------------
// UI helpers (inline styles — Phase 1 keeps the dep surface tiny; Phase 3 will
// migrate to MUI per spec 003 §8.1).
// ---------------------------------------------------------------------------
const styles = {
  page: { padding: '1.5rem', fontFamily: 'system-ui, sans-serif' } as const,
  h1: { margin: '0 0 1rem', fontSize: '1.5rem' } as const,
  bar: {
    display: 'flex',
    flexWrap: 'wrap' as const,
    gap: '0.75rem',
    alignItems: 'flex-end',
    padding: '1rem',
    background: '#f5f7fa',
    borderRadius: 8,
    marginBottom: '1rem',
  },
  field: { display: 'flex', flexDirection: 'column' as const, gap: 4 },
  label: { fontSize: '0.75rem', color: '#555' } as const,
  input: {
    padding: '0.4rem 0.6rem',
    border: '1px solid #ccc',
    borderRadius: 4,
    fontSize: '0.875rem',
    minWidth: 180,
  } as const,
  button: {
    padding: '0.45rem 0.9rem',
    border: '1px solid #1976d2',
    background: '#1976d2',
    color: '#fff',
    borderRadius: 4,
    cursor: 'pointer',
    fontSize: '0.875rem',
  } as const,
  buttonGhost: {
    padding: '0.45rem 0.9rem',
    border: '1px solid #ccc',
    background: '#fff',
    color: '#333',
    borderRadius: 4,
    cursor: 'pointer',
    fontSize: '0.875rem',
  } as const,
  table: {
    width: '100%',
    borderCollapse: 'collapse' as const,
    fontSize: '0.85rem',
    background: '#fff',
  },
  th: {
    textAlign: 'left' as const,
    padding: '0.5rem 0.75rem',
    borderBottom: '2px solid #e0e0e0',
    background: '#fafbfc',
    position: 'sticky' as const,
    top: 0,
  },
  td: { padding: '0.5rem 0.75rem', borderBottom: '1px solid #eee', verticalAlign: 'top' as const },
  rowHover: { cursor: 'pointer' as const },
  pill: {
    display: 'inline-block',
    padding: '0.1rem 0.5rem',
    background: '#eef2ff',
    color: '#3730a3',
    borderRadius: 12,
    fontSize: '0.75rem',
    fontFamily: 'ui-monospace, monospace',
  } as const,
  drawer: {
    position: 'fixed' as const,
    top: 0,
    right: 0,
    bottom: 0,
    width: '50%',
    minWidth: 480,
    background: '#fff',
    boxShadow: '-8px 0 24px rgba(0,0,0,0.12)',
    padding: '1.5rem',
    overflow: 'auto' as const,
    zIndex: 1000,
  },
  drawerClose: {
    position: 'absolute' as const,
    top: 12,
    right: 16,
    border: 'none',
    background: 'transparent',
    fontSize: '1.5rem',
    cursor: 'pointer',
  } as const,
  pre: {
    background: '#0d1117',
    color: '#c9d1d9',
    padding: '1rem',
    borderRadius: 6,
    overflow: 'auto' as const,
    fontSize: '0.75rem',
    fontFamily: 'ui-monospace, monospace',
  } as const,
  banner: (kind: 'info' | 'error') =>
    ({
      padding: '0.5rem 0.75rem',
      borderRadius: 4,
      marginBottom: '0.75rem',
      fontSize: '0.85rem',
      background: kind === 'error' ? '#fdecea' : '#e3f2fd',
      color: kind === 'error' ? '#b71c1c' : '#0d47a1',
    } as const),
};

function fmtTime(iso: string): string {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// ---------------------------------------------------------------------------
// Audit log page
// ---------------------------------------------------------------------------
const AuditPage = (_props: PluginRouteProps) => {
  const [filter, setFilter] = React.useState<FilterState>(emptyFilter);
  const [draft, setDraft] = React.useState<FilterState>(emptyFilter);
  const [events, setEvents] = React.useState<AuditEvent[]>([]);
  const [nextBeforeID, setNextBeforeID] = React.useState<number | null>(null);
  const [types, setTypes] = React.useState<string[]>([]);
  const [namespaces, setNamespaces] = React.useState<string[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [selected, setSelected] = React.useState<AuditEvent | null>(null);

  // Initial load of distinct values for filter dropdowns.
  React.useEffect(() => {
    api<{ items: string[] | null }>('/events/types')
      .then((r) => setTypes(r.items ?? []))
      .catch(() => {});
    api<{ items: string[] | null }>('/events/namespaces')
      .then((r) => setNamespaces(r.items ?? []))
      .catch(() => {});
  }, []);

  // Reload events whenever the committed filter changes.
  React.useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api<ListResponse>('/events' + buildQuery(filter))
      .then((r) => {
        if (cancelled) return;
        setEvents(r.items ?? []);
        setNextBeforeID(r.nextBeforeID ?? null);
      })
      .catch((err: Error) => !cancelled && setError(err.message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [filter]);

  const apply = () => setFilter({ ...draft, beforeID: null });
  const reset = () => {
    setDraft(emptyFilter);
    setFilter(emptyFilter);
  };
  const loadMore = () => {
    if (nextBeforeID == null) return;
    setLoading(true);
    api<ListResponse>('/events' + buildQuery({ ...filter, beforeID: nextBeforeID }))
      .then((r) => {
        setEvents((prev) => [...prev, ...(r.items ?? [])]);
        setNextBeforeID(r.nextBeforeID ?? null);
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  };

  return React.createElement(
    'div',
    { style: styles.page },
    React.createElement('h1', { style: styles.h1 }, '📜 Audit Log'),

    // Filter bar
    React.createElement(
      'div',
      { style: styles.bar },
      React.createElement(
        'div',
        { style: styles.field },
        React.createElement('label', { style: styles.label }, 'Event types'),
        React.createElement(MultiSelect, {
          options: types,
          value: draft.types,
          onChange: (v: string[]) => setDraft({ ...draft, types: v }),
          placeholder: 'All types',
        })
      ),
      React.createElement(
        'div',
        { style: styles.field },
        React.createElement('label', { style: styles.label }, 'Namespaces'),
        React.createElement(MultiSelect, {
          options: namespaces,
          value: draft.namespaces,
          onChange: (v: string[]) => setDraft({ ...draft, namespaces: v }),
          placeholder: 'All namespaces',
        })
      ),
      React.createElement(
        'div',
        { style: styles.field },
        React.createElement('label', { style: styles.label }, 'Search'),
        React.createElement('input', {
          style: styles.input,
          type: 'text',
          placeholder: 'name or payload…',
          value: draft.search,
          onChange: (e: any) => setDraft({ ...draft, search: e.target.value }),
          onKeyDown: (e: any) => e.key === 'Enter' && apply(),
        })
      ),
      React.createElement(
        'div',
        { style: styles.field },
        React.createElement('label', { style: styles.label }, 'From'),
        React.createElement('input', {
          style: styles.input,
          type: 'datetime-local',
          value: draft.since,
          onChange: (e: any) => setDraft({ ...draft, since: e.target.value }),
        })
      ),
      React.createElement(
        'div',
        { style: styles.field },
        React.createElement('label', { style: styles.label }, 'To'),
        React.createElement('input', {
          style: styles.input,
          type: 'datetime-local',
          value: draft.until,
          onChange: (e: any) => setDraft({ ...draft, until: e.target.value }),
        })
      ),
      React.createElement('button', { style: styles.button, onClick: apply }, 'Apply'),
      React.createElement('button', { style: styles.buttonGhost, onClick: reset }, 'Reset')
    ),

    error && React.createElement('div', { style: styles.banner('error') }, `✗ ${error}`),
    loading && events.length === 0
      ? React.createElement('div', { style: styles.banner('info') }, 'Loading…')
      : events.length === 0
      ? React.createElement(
          'div',
          { style: styles.banner('info') },
          'No events yet. Once the plugin daemon connects to /v1/events, captured events will appear here.'
        )
      : null,

    // Event table
    events.length > 0 &&
      React.createElement(
        'table',
        { style: styles.table },
        React.createElement(
          'thead',
          null,
          React.createElement(
            'tr',
            null,
            React.createElement('th', { style: styles.th }, 'Time'),
            React.createElement('th', { style: styles.th }, 'Type'),
            React.createElement('th', { style: styles.th }, 'Namespace'),
            React.createElement('th', { style: styles.th }, 'Resource'),
            React.createElement('th', { style: styles.th }, 'Actor')
          )
        ),
        React.createElement(
          'tbody',
          null,
          events.map((e) =>
            React.createElement(
              'tr',
              {
                key: e.id,
                style: styles.rowHover,
                onClick: () => setSelected(e),
              },
              React.createElement('td', { style: styles.td }, fmtTime(e.occurredAt)),
              React.createElement(
                'td',
                { style: styles.td },
                React.createElement('span', { style: styles.pill }, e.type)
              ),
              React.createElement('td', { style: styles.td }, e.namespace ?? '—'),
              React.createElement(
                'td',
                { style: styles.td },
                e.resourceKind
                  ? `${e.resourceKind}/${e.resourceName ?? ''}`
                  : '—'
              ),
              React.createElement(
                'td',
                { style: styles.td },
                e.actorID ? `${e.actorType ?? ''}:${e.actorID}` : '—'
              )
            )
          )
        )
      ),

    nextBeforeID != null &&
      React.createElement(
        'div',
        { style: { marginTop: '1rem', textAlign: 'center' as const } },
        React.createElement(
          'button',
          { style: styles.buttonGhost, onClick: loadMore, disabled: loading },
          loading ? 'Loading…' : 'Load more'
        )
      ),

    selected && React.createElement(EventDrawer, { event: selected, onClose: () => setSelected(null) })
  );
};

// ---------------------------------------------------------------------------
// Detail drawer
// ---------------------------------------------------------------------------
const EventDrawer = (props: { event: AuditEvent; onClose: () => void }) => {
  const { event, onClose } = props;
  return React.createElement(
    'div',
    { style: styles.drawer },
    React.createElement('button', { style: styles.drawerClose, onClick: onClose, 'aria-label': 'Close' }, '×'),
    React.createElement('h2', { style: { marginTop: 0 } }, event.type),
    React.createElement('p', { style: { color: '#555', marginTop: 0 } }, fmtTime(event.occurredAt)),
    React.createElement(
      'dl',
      { style: { display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '0.4rem 1rem', fontSize: '0.875rem' } },
      labelValue('Namespace', event.namespace),
      labelValue('Resource', event.resourceKind ? `${event.resourceKind}/${event.resourceName ?? ''}` : undefined),
      labelValue('Actor', event.actorID ? `${event.actorType ?? ''}:${event.actorID}` : undefined),
      labelValue('Resource version', event.resourceVersion)
    ),
    React.createElement('h3', null, 'Envelope'),
    React.createElement('pre', { style: styles.pre }, JSON.stringify(event.envelope, null, 2))
  );
};

function labelValue(label: string, value: string | undefined) {
  if (!value) return null;
  return [
    React.createElement('dt', { key: `${label}-l`, style: { color: '#666' } }, label),
    React.createElement('dd', { key: `${label}-v`, style: { margin: 0 } }, value),
  ];
}

// ---------------------------------------------------------------------------
// MultiSelect — minimal native <select multiple> wrapper
// ---------------------------------------------------------------------------
const MultiSelect = (props: {
  options: string[];
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
}) => {
  return React.createElement(
    'select',
    {
      multiple: true,
      style: { ...styles.input, minHeight: 80, minWidth: 220 },
      value: props.value,
      onChange: (e: any) => {
        const v = Array.from(e.target.selectedOptions, (o: any) => o.value as string);
        props.onChange(v);
      },
    },
    props.options.length === 0
      ? React.createElement('option', { disabled: true, value: '' }, props.placeholder ?? '—')
      : props.options.map((o) => React.createElement('option', { key: o, value: o }, o))
  );
};

// ---------------------------------------------------------------------------
// Plugin registration
// ---------------------------------------------------------------------------
const register: PluginRegisterFn = (api: PluginApi) => {
  React = api.React;
  pluginFetch = api.fetch.bind(api);

  api.registerExtension({
    type: 'sidebarItem',
    label: 'Audit Log',
  });

  api.registerExtension({
    type: 'route',
    label: 'Audit Log',
    component: AuditPage,
  });
};

export default register;
