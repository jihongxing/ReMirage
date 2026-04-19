import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { Gateways } from './pages/Gateways';
import { Cells } from './pages/Cells';
import { Billing } from './pages/Billing';
import { Threats } from './pages/Threats';
import { Strategy } from './pages/Strategy';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/gateways" element={<Gateways />} />
          <Route path="/cells" element={<Cells />} />
          <Route path="/billing" element={<Billing />} />
          <Route path="/threats" element={<Threats />} />
          <Route path="/strategy" element={<Strategy />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
