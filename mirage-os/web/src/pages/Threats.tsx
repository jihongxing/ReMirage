import { useState } from 'react';
import { useApi } from '../hooks/useApi';
import { DataTable } from '../components/DataTable';
import { StatusIndicator } from '../components/StatusIndicator';
import { ControlPanel } from '../components/ControlPanel';

interface Threat {
  source_ip: string; threat_type: string; severity: number;
  hit_count: number; is_banned: boolean; last_seen: string;
}

export function Threats() {
  const [typeFilter, setTypeFilter] = useState('ALL');
  const [banFilter, setBanFilter] = useState('ALL');
  const { data, loading } = useApi<Threat[]>('/threats');

  const filtered = (data ?? []).filter(t =>
    (typeFilter === 'ALL' || t.threat_type === typeFilter) &&
    (banFilter === 'ALL' || (banFilter === 'BANNED' ? t.is_banned : !t.is_banned))
  );

  const threatTypes = [...new Set(data?.map(t => t.threat_type) ?? [])];

  const columns = [
    { key: 'source_ip', title: '源 IP' },
    { key: 'threat_type', title: '威胁类型' },
    { key: 'severity', title: '严重程度' },
    { key: 'hit_count', title: '命中次数' },
    { key: 'is_banned', title: '封禁状态', render: (_: unknown, r: Threat) =>
      <StatusIndicator status={r.is_banned ? 'offline' : 'online'} label={r.is_banned ? '已封禁' : '未封禁'} />
    },
    { key: 'last_seen', title: '最后发现', render: (v: unknown) => new Date(v as string).toLocaleString() },
  ];

  return (
    <div>
      <h2 className="text-xl font-semibold mb-4">Threats</h2>
      <div className="mb-4">
        <ControlPanel controls={[
          {
            type: 'select', label: '威胁类型', value: typeFilter,
            options: [{ value: 'ALL', label: '全部' }, ...threatTypes.map(t => ({ value: t, label: t }))],
            onChange: setTypeFilter,
          },
          {
            type: 'select', label: '封禁状态', value: banFilter,
            options: [{ value: 'ALL', label: '全部' }, { value: 'BANNED', label: '已封禁' }, { value: 'ACTIVE', label: '未封禁' }],
            onChange: setBanFilter,
          },
        ]} />
      </div>
      <DataTable columns={columns} data={filtered} loading={loading} />
    </div>
  );
}
