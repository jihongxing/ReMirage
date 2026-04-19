interface StatusIndicatorProps {
  status: 'online' | 'degraded' | 'offline';
  label?: string;
}

const config = {
  online:   { emoji: '🟢', text: '在线', cls: 'text-green-400' },
  degraded: { emoji: '🟡', text: '降级', cls: 'text-yellow-400' },
  offline:  { emoji: '🔴', text: '离线', cls: 'text-red-400' },
};

export function StatusIndicator({ status, label }: StatusIndicatorProps) {
  const c = config[status] || config.offline;
  return (
    <span className={`inline-flex items-center gap-1 ${c.cls}`}>
      <span>{c.emoji}</span>
      <span className="text-sm">{label ?? c.text}</span>
    </span>
  );
}
