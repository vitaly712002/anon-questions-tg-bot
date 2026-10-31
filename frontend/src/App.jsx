import React, { useState, useEffect } from 'react';
import Navbar from './components/Navbar';
import StatCard from './components/StatCard';
import QuestionsFeed from './components/QuestionsFeed';
import UsersTable from './components/UsersTable';
import SettingsPanel from './components/SettingsPanel';
import LoginModal from './components/LoginModal';
import { getCurrentUser, logout, getStats, getQuestions, getUsers } from './api';
import { Users, MessageSquare, CheckCircle, Ban, ArrowUpRight } from 'lucide-react';

export default function App() {
  const [user, setUser] = useState(getCurrentUser());
  const [activeTab, setActiveTab] = useState('dashboard');
  const [stats, setStats] = useState(null);
  const [questions, setQuestions] = useState([]);
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);

  const loadData = async () => {
    if (!user) return;
    try {
      setLoading(true);
      const [st, q, u] = await Promise.all([
        getStats().catch(() => null),
        getQuestions(50, 0).catch(() => []),
        getUsers().catch(() => []),
      ]);
      if (st) setStats(st);
      if (Array.isArray(q)) setQuestions(q);
      if (Array.isArray(u)) setUsers(u);
    } catch (err) {
      console.error('Ошибка загрузки данных', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (user) {
      loadData();
    }
  }, [user]);

  const handleLogout = () => {
    logout();
    setUser(null);
  };

  if (!user) {
    return <LoginModal onLoginSuccess={(u) => setUser(u)} />;
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col">
      <Navbar
        activeTab={activeTab}
        setActiveTab={setActiveTab}
        user={user}
        onLogout={handleLogout}
      />

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {activeTab === 'dashboard' && (
          <div className="space-y-8">
            {/* Карточки метрик */}
            <div>
              <h2 className="text-xl font-bold text-white mb-1">Обзор системы</h2>
              <p className="text-sm text-slate-400 mb-6">Ключевые показатели работы Telegram-бота</p>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard
                  title="Всего пользователей"
                  value={stats?.total_users}
                  icon={Users}
                  color="indigo"
                  subtitle="Зарегистрировано в боте"
                />
                <StatCard
                  title="Задано вопросов"
                  value={stats?.total_questions}
                  icon={MessageSquare}
                  color="blue"
                  subtitle="За всё время"
                />
                <StatCard
                  title="Ответов получено"
                  value={stats?.answered_count}
                  icon={CheckCircle}
                  color="green"
                  subtitle="Успешных диалогов"
                />
                <StatCard
                  title="Заблокировано"
                  value={stats?.banned_users}
                  icon={Ban}
                  color="red"
                  subtitle="Нарушителей правил"
                />
              </div>
            </div>

            {/* Последние вопросы (мини-виджет) */}
            <div className="bg-slate-900/60 border border-slate-800 rounded-2xl p-6 shadow-sm">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <h3 className="font-bold text-white text-base">Свежие вопросы</h3>
                  <p className="text-xs text-slate-400">Последние поступившие сообщения</p>
                </div>
                <button
                  onClick={() => setActiveTab('questions')}
                  className="flex items-center space-x-1 text-xs text-indigo-400 hover:text-indigo-300 font-medium"
                >
                  <span>Все вопросы</span>
                  <ArrowUpRight className="w-3.5 h-3.5" />
                </button>
              </div>

              {questions.length === 0 ? (
                <div className="py-8 text-center text-slate-500 text-sm">
                  Пока нет принятых вопросов
                </div>
              ) : (
                <div className="divide-y divide-slate-800/60">
                  {questions.slice(0, 5).map((q) => (
                    <div key={q.id} className="py-3 flex items-center justify-between text-sm">
                      <div className="flex items-center space-x-3">
                        <span className="text-amber-400 font-medium">
                          {q.sender_username || 'Аноним'}
                        </span>
                        <span className="text-slate-600">➔</span>
                        <span className="text-indigo-400 font-medium">
                          @{q.recipient_username || q.recipient_name}
                        </span>
                        <span className="text-xs text-slate-500">
                          ({q.content_type})
                        </span>
                      </div>
                      <div className="text-xs text-slate-500">
                        {new Date(q.created_at).toLocaleTimeString('ru-RU', {
                          hour: '2-digit',
                          minute: '2-digit',
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {activeTab === 'questions' && (
          <QuestionsFeed
            questions={questions}
            loading={loading}
            onRefresh={loadData}
          />
        )}

        {activeTab === 'users' && (
          <UsersTable
            users={users}
            loading={loading}
            onRefresh={loadData}
          />
        )}

        {activeTab === 'settings' && <SettingsPanel />}
      </main>
    </div>
  );
}
