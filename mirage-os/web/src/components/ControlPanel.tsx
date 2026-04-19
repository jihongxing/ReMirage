interface SelectControl {
  type: 'select';
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}

interface ButtonControl {
  type: 'button';
  label: string;
  onClick: () => void;
  variant?: 'primary' | 'danger';
}

type Control = SelectControl | ButtonControl;

interface ControlPanelProps {
  controls: Control[];
}

export function ControlPanel({ controls }: ControlPanelProps) {
  return (
    <div className="flex items-center gap-4 flex-wrap">
      {controls.map((ctrl, idx) => {
        if (ctrl.type === 'select') {
          return (
            <label key={idx} className="flex items-center gap-2 text-sm text-slate-300">
              {ctrl.label}
              <select
                value={ctrl.value}
                onChange={e => ctrl.onChange(e.target.value)}
                className="bg-slate-700 border border-slate-600 rounded px-3 py-1.5 text-white text-sm focus:outline-none focus:ring-1 focus:ring-cyan-500"
              >
                {ctrl.options.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </label>
          );
        }
        const btnCls = ctrl.variant === 'danger'
          ? 'bg-red-600 hover:bg-red-700'
          : 'bg-cyan-600 hover:bg-cyan-700';
        return (
          <button
            key={idx}
            onClick={ctrl.onClick}
            className={`${btnCls} text-white px-4 py-1.5 rounded text-sm transition-colors`}
          >
            {ctrl.label}
          </button>
        );
      })}
    </div>
  );
}
