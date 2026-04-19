// 影子页面 - 展示给审计者的伪装页面
import { useState, useEffect } from 'react';

const ShadowCover = () => {
  const [time, setTime] = useState(new Date());

  useEffect(() => {
    const timer = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 flex items-center justify-center">
      <div className="text-center">
        {/* 伪装成普通企业页面 */}
        <div className="mb-8">
          <div className="text-6xl mb-4">🌐</div>
          <h1 className="text-4xl font-light text-white mb-2">CloudSync Solutions</h1>
          <p className="text-slate-400">Enterprise Infrastructure Management</p>
        </div>

        {/* 假的维护信息 */}
        <div className="bg-slate-800/50 backdrop-blur-sm rounded-lg p-8 max-w-md mx-auto border border-slate-700">
          <div className="flex items-center justify-center gap-2 mb-4">
            <div className="w-3 h-3 bg-yellow-500 rounded-full animate-pulse" />
            <span className="text-yellow-500 font-medium">Scheduled Maintenance</span>
          </div>
          
          <p className="text-slate-300 mb-6">
            Our systems are currently undergoing scheduled maintenance to improve performance and security.
          </p>

          <div className="text-slate-500 text-sm">
            <p>Expected completion: {time.toLocaleTimeString()}</p>
            <p className="mt-2">Reference ID: {Math.random().toString(36).substring(2, 10).toUpperCase()}</p>
          </div>
        </div>

        {/* 假的联系信息 */}
        <div className="mt-8 text-slate-500 text-sm">
          <p>For urgent inquiries: support@cloudsync-solutions.com</p>
          <p className="mt-1">© 2026 CloudSync Solutions Ltd. All rights reserved.</p>
        </div>
      </div>
    </div>
  );
};

export default ShadowCover;
