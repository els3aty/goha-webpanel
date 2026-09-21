import React, { useState } from 'react';
import { Routes, Route, Link, useLocation } from 'react-router-dom';
import { 
  LayoutDashboard, 
  Globe, 
  Database, 
  Settings, 
  LogOut,
  Menu,
  X,
  Server
} from 'lucide-react';
import './Dashboard.css';

const Dashboard: React.FC = () => {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const location = useLocation();

  const toggleSidebar = () => setSidebarOpen(!sidebarOpen);

  const menuItems = [
    { path: '/dashboard', icon: LayoutDashboard, label: 'Overview' },
    { path: '/dashboard/websites', icon: Globe, label: 'Websites' },
    { path: '/dashboard/databases', icon: Database, label: 'Databases' },
    { path: '/dashboard/settings', icon: Settings, label: 'Settings' },
  ];

  return (
    <div className="dashboard-container">
      {/* Sidebar */}
      <aside className={`sidebar glass-panel ${sidebarOpen ? 'open' : 'closed'}`}>
        <div className="sidebar-header">
          <div className="logo-container-small">
            <Server size={24} className="logo-icon" />
          </div>
          {sidebarOpen && <span className="sidebar-title">Goha<span className="text-gradient">Host</span></span>}
        </div>

        <nav className="sidebar-nav">
          {menuItems.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname === item.path;
            
            return (
              <Link 
                key={item.path} 
                to={item.path}
                className={`nav-item ${isActive ? 'active' : ''}`}
                title={!sidebarOpen ? item.label : undefined}
              >
                <Icon size={20} className="nav-icon" />
                {sidebarOpen && <span className="nav-label">{item.label}</span>}
              </Link>
            );
          })}
        </nav>

        <div className="sidebar-footer">
          <button className="nav-item logout-btn" title={!sidebarOpen ? 'Logout' : undefined}>
            <LogOut size={20} className="nav-icon" />
            {sidebarOpen && <span className="nav-label">Logout</span>}
          </button>
        </div>
      </aside>

      {/* Main Content */}
      <main className="main-content">
        {/* Top Navigation */}
        <header className="topbar glass-panel">
          <button onClick={toggleSidebar} className="icon-btn">
            {sidebarOpen ? <X size={24} /> : <Menu size={24} />}
          </button>
          
          <div className="topbar-right">
            <div className="user-profile">
              <div className="avatar">A</div>
              <span className="username">Admin</span>
            </div>
          </div>
        </header>

        {/* Page Content */}
        <div className="page-content animate-fade-in">
          <Routes>
            <Route path="/" element={
              <div className="overview-content">
                <h1 className="page-title">Overview</h1>
                <p className="page-subtitle">Welcome back to GohaHost Control Panel.</p>
                
                <div className="stats-grid">
                  <div className="stat-card glass-panel">
                    <h3>Total Websites</h3>
                    <div className="stat-value text-gradient">0</div>
                  </div>
                  <div className="stat-card glass-panel">
                    <h3>Active Databases</h3>
                    <div className="stat-value text-gradient">0</div>
                  </div>
                  <div className="stat-card glass-panel">
                    <h3>Server Load</h3>
                    <div className="stat-value text-gradient">1.2%</div>
                  </div>
                </div>
              </div>
            } />
            <Route path="/websites" element={<h1 className="page-title">Websites Management (Coming Soon)</h1>} />
            <Route path="/databases" element={<h1 className="page-title">Database Management (Coming Soon)</h1>} />
            <Route path="/settings" element={<h1 className="page-title">Settings (Coming Soon)</h1>} />
          </Routes>
        </div>
      </main>
    </div>
  );
};

export default Dashboard;
