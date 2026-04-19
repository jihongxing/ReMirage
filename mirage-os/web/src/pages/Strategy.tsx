import { useState } from 'react';
import { useApi, apiPost } from '../hooks/useApi';
import { ControlPanel } from '../components/ControlPanel';

interface Cell { id: string; name: string }

const levels = [0, 1, 2, 3, 4].map(l => ({ value: String(l), label: `Level ${l}` }));
const templates = [
  { value: 'zoom', label: 'Zoom' },
  { value: 'chrome', label: 'Chrome' },
  { value: 'teams', label: 'Teams' },
];

export function Strategy() {
  const { data: cells } = useApi<Cell[]>('/cells');
  const [cellId, setCellId] = useState('');
  const [level, setLevel] = useState('2');
  const [template, setTemplate] = useState('zoom');
  const [msg, setMsg] = useState('');

  const handleApply = async () => {
    if (!cellId) { setMsg('请选择蜂窝'); return; }
    try {
      await apiPost('/strategy/apply', {
        cell_id: cellId,
        defense_level: parseInt(level),
        template_id: template,
      });
      setMsg('策略应用成功');
    } catch {
      setMsg('策略应用失败');
    }
  };

  const cellOptions = [
    { value: '', label: '选择蜂窝' },
    ...(cells ?? []).map(c => ({ value: c.id, label: c.name })),
  ];

  return (
    <div>
      <h2 className="text-xl font-semibold mb-4">Strategy</h2>
      <div className="bg-slate-900 border border-slate-800 rounded-lg p-6 max-w-xl">
        <div className="space-y-4">
          <ControlPanel controls={[
            { type: 'select', label: '蜂窝', value: cellId, options: cellOptions, onChange: setCellId },
          ]} />
          <ControlPanel controls={[
            { type: 'select', label: '防御等级', value: level, options: levels, onChange: setLevel },
          ]} />
          <ControlPanel controls={[
            { type: 'select', label: '拟态模板', value: template, options: templates, onChange: setTemplate },
          ]} />
          <ControlPanel controls={[
            { type: 'button', label: '应用策略', onClick: handleApply, variant: 'primary' },
          ]} />
          {msg && (
            <p className={`text-sm ${msg.includes('成功') ? 'text-green-400' : 'text-red-400'}`}>{msg}</p>
          )}
        </div>
      </div>
    </div>
  );
}
