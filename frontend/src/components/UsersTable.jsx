import React, { useState } from 'react';
import { Search, Shield, Ban, CheckCircle, ExternalLink } from 'lucide-react';
import { updateUserRole, toggleUserBan } from '../api';

export default function UsersTable({ users, loading, onRefresh }) {
  const [search, setSearch] = useState('');
  const [updatingId, setUpdatingId] = useState(null);

  const filtered = (users || []).filter((u) => {
    if (!search.trim()) return true;
    const s = search.toLowerCase();
    return (
      u.tg_username?.toLowerCase().includes(s) ||
      u.display_name?.toLowerCase().includes(s) ||
      String(u.tg_id).includes(s) ||
      u.link_alias?.toLowerCase().includes(s)
    );
  });

  const handleRoleChange = async (userId, newRole) => {
    try {
      setUpdatingId(userId);
      await updateUserRole(userId, newRole);
      onRefresh();
    } catch (err) {
      alert('Ошибка изменения роли: ' + err.message);
    } finally {
      setUpdatingId(null);
    }
  };

  const handleBanToggle = async (userId, currentBanned) => {
    try {
      setUpdatingId(userId);
      await toggleUserBan(userId, !currentBanned);
      onRefresh();
    } catch (err) {
      alert('Ошибка блокировки: ' + err.message);
    } finally {
      setUpdatingId(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white">Управление пользователями</h2>
          <p className="text-sm text-slate-400">
            Назначение ролей, просмотр ссылок и глобальная блокировка
          </p>
        </div>
        <button
          onClick={onRefresh}
          className="self-start sm:self-auto px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-sm font-medium rounded-xl border border-slate-700 transition-colors"
        >
          Обновить
        </button>
      </div>

      <div className="relative">
        <Search className="w-4 h-4 absolute left-3.5 top-3 text-slate-500" />
        <input
          type="text"
          placeholder="Поиск по юзернейму, имени или Telegram ID..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-10 pr-4 py-2 bg-slate-900/60 border border-slate-800 rounded-xl text-sm text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500"
        />
      </div>

      <div className="bg-slate-900/70 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="bg-slate-950/60 text-xs uppercase text-slate-400 border-b border-slate-800">
              <tr>
                <th className="px-6 py-4">Пользователь</th>
                <th className="px-6 py-4">Telegram ID</th>
                <th className="px-6 py-4">Ссылка / Алиас</th>
                <th className="px-6 py-4">Вопросов</th>
                <th className="px-6 py-4">Роль</th>
                <th className="px-6 py-4">Статус</th>
                <th className="px-6 py-4 text-right">Действия</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {loading ? (
                <tr>
                  <td colSpan="7" className="px-6 py-12 text-center text-slate-500">
                    Загрузка пользователей...
                  </td>
                </tr>
              ) : filtered.length === 0 ? (
                <tr>
                  <td colSpan="7" className="px-6 py-12 text-center text-slate-500">
                    Пользователи не найдены
                  </td>
                </tr>
              ) : (
                filtered.map((u) => (
                  <tr key={u.id} className="hover:bg-slate-800/30 transition-colors">
                    <td className="px-6 py-4">
                      <div className="font-medium text-white">{u.display_name || 'Без имени'}</div>
                      <div className="text-xs text-slate-400">
                        {u.tg_username ? `@${u.tg_username}` : 'нет юзернейма'}
                      </div>
                    </td>
                    <td className="px-6 py-4 font-mono text-xs text-slate-400">{u.tg_id}</td>
                    <td className="px-6 py-4">
                      {u.link_alias ? (
                        <span className="text-indigo-400 bg-indigo-500/10 px-2 py-0.5 rounded text-xs">
                          @{u.link_alias}
                        </span>
                      ) : u.link_token ? (
                        <span className="font-mono text-xs text-slate-400">
                          q_{u.link_token.substring(0, 8)}...
                        </span>
                      ) : (
                        <span className="text-slate-600 text-xs">нет ссылки</span>
                      )}
                    </td>
                    <td className="px-6 py-4">
                      <span className="font-semibold text-slate-200">{u.questions_count}</span>
                    </td>
                    <td className="px-6 py-4">
                      <select
                        value={u.role}
                        disabled={updatingId === u.id}
                        onChange={(e) => handleRoleChange(u.id, e.target.value)}
                        className="bg-slate-950 border border-slate-700 text-xs rounded-lg px-2.5 py-1 text-slate-200 focus:outline-none focus:border-indigo-500 capitalize"
                      >
                        <option value="user">User</option>
                        <option value="admin">Admin</option>
                        <option value="sysadmin">Sysadmin</option>
                      </select>
                    </td>
                    <td className="px-6 py-4">
                      {u.banned ? (
                        <span className="inline-flex items-center space-x-1 text-rose-400 bg-rose-500/10 px-2 py-0.5 rounded text-xs">
                          <Ban className="w-3 h-3" />
                          <span>Забанен</span>
                        </span>
                      ) : (
                        <span className="inline-flex items-center space-x-1 text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded text-xs">
                          <CheckCircle className="w-3 h-3" />
                          <span>Активен</span>
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 text-right">
                      <button
                        disabled={updatingId === u.id}
                        onClick={() => handleBanToggle(u.id, u.banned)}
                        className={`text-xs px-3 py-1 rounded-lg font-medium transition-colors ${
                          u.banned
                            ? 'bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20'
                            : 'bg-rose-500/10 text-rose-400 hover:bg-rose-500/20'
                        }`}
                      >
                        {u.banned ? 'Разбанить' : 'Заблокировать'}
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
