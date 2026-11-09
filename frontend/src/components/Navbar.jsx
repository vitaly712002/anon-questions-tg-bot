import React from 'react';
import { MessageSquare, Users, Settings, BarChart2, LogOut, Shield, Link2 } from 'lucide-react';

export default function Navbar({ activeTab, setActiveTab, user, onLogout }) {
  const tabs = [
    { id: 'dashboard', label: 'Обзор', icon: BarChart2 },
    { id: 'questions', label: 'Вопросы (Feed)', icon: MessageSquare },
    { id: 'visits', label: 'Переходы по ссылкам', icon: Link2 },
    { id: 'users', label: 'Пользователи', icon: Users },
    { id: 'settings', label: 'Настройки бота', icon: Settings },
  ];

  return (
    <header className="border-b border-slate-800 bg-slate-900/50 backdrop-blur sticky top-0 z-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex items-center justify-between h-16">
        <div className="flex items-center space-x-3">
          <div className="w-10 h-10 rounded-xl bg-indigo-600 flex items-center justify-center text-white shadow-lg shadow-indigo-500/20">
            <Shield className="w-5 h-5" />
          </div>
          <div>
            <h1 className="font-bold text-lg text-white leading-tight">Secure Ask</h1>
            <p className="text-xs text-slate-400">Панель управления</p>
          </div>
        </div>

        <nav className="flex space-x-1 sm:space-x-2">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center space-x-2 px-3 py-2 rounded-lg text-sm font-medium transition-all ${
                  isActive
                    ? 'bg-indigo-600/10 text-indigo-400 border border-indigo-500/30'
                    : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
                }`}
              >
                <Icon className="w-4 h-4" />
                <span className="hidden md:inline">{tab.label}</span>
              </button>
            );
          })}
        </nav>

        <div className="flex items-center space-x-4">
          <div className="text-right hidden sm:block">
            <div className="text-sm font-medium text-slate-200">{user?.username}</div>
            <div className="text-xs text-indigo-400 capitalize">{user?.role}</div>
          </div>
          <button
            onClick={onLogout}
            title="Выйти"
            className="p-2 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
          >
            <LogOut className="w-5 h-5" />
          </button>
        </div>
      </div>
    </header>
  );
}
