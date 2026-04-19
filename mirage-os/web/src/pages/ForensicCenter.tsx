// 取证归档中心 - 整合取证相关组件
import ForensicArchive from '../components/ForensicArchive';
import GlobalAuditTrail from '../components/GlobalAuditTrail';

export default function ForensicCenter() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-white flex items-center gap-2">
        🔍 取证归档中心
      </h1>
      
      <div className="grid grid-cols-12 gap-6">
        <div className="col-span-7">
          <ForensicArchive />
        </div>
        <div className="col-span-5">
          <GlobalAuditTrail />
        </div>
      </div>
    </div>
  );
}
